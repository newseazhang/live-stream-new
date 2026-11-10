import type { ClientDanma, ServerMessage } from './websocket';

export type DeliveryState = 'pending' | 'accepted' | 'unknown' | 'rejected';
export interface OutgoingDanma {
  readonly message: ClientDanma;
  readonly state: DeliveryState;
  readonly attempts: number;
  readonly reason: string;
}

// 当前房间和当前身份的内存记录，保留至确认、主动移除或页面销毁。
export class DanmaOutbox {
  private entries = new Map<string, OutgoingDanma>();
  private timers = new Map<string, ReturnType<typeof setTimeout>>();
  private listeners = new Set<() => void>();

  constructor(private roomID: number, private userID: number | null, private timeoutMS = 8000) {}

  snapshot(): OutgoingDanma[] { return [...this.entries.values()]; }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  private emit() { for (const listener of this.listeners) listener(); }

  private clearTimer(id: string) {
    const timer = this.timers.get(id);
    if (timer !== undefined) clearTimeout(timer);
    this.timers.delete(id);
  }

  private begin(id: string): ClientDanma {
    const entry = this.entries.get(id)!;
    this.clearTimer(id);
    this.entries.set(id, { ...entry, state: 'pending', attempts: entry.attempts + 1, reason: '' });
    this.timers.set(id, setTimeout(() => {
      this.unknown(id, '尚未收到保存确认，结果未知；可以使用原消息重试。');
    }, this.timeoutMS));
    this.emit();
    return entry.message;
  }

  add(message: ClientDanma): ClientDanma {
    if (this.userID === null) throw new Error('请先登录后发送弹幕。');
    if (this.entries.size >= 50) throw new Error('待处理弹幕已达 50 条，请先确认或移除记录。');
    if (this.entries.has(message.message_id)) throw new Error('该消息已经在待处理列表中。');
    this.entries.set(message.message_id, {
      message: Object.freeze({ ...message }), state: 'unknown', attempts: 0, reason: '',
    });
    return this.begin(message.message_id);
  }

  retry(id: string): ClientDanma | null {
    if (this.entries.get(id)?.state !== 'unknown') return null;
    return this.begin(id);
  }

  accept(message: ServerMessage): boolean {
    if (message.type !== 'accepted' || message.room_id !== this.roomID ||
        message.user_id !== this.userID || !message.message_id) return false;
    const id = message.message_id;
    const entry = this.entries.get(id);
    if (!entry || entry.state === 'rejected' || entry.state === 'accepted' ||
        entry.message.content !== message.content) return false;
    this.clearTimer(id);
    this.entries.set(id, { ...entry, state: 'accepted', reason: '' });
    this.timers.set(id, setTimeout(() => {
      this.unknown(id, '消息已接收，但保存尚未确认；可以使用原消息重试。');
    }, this.timeoutMS));
    this.emit();
    return true;
  }

  unknown(id: string, reason: string) {
    const entry = this.entries.get(id);
    if (!entry || entry.state === 'rejected') return;
    this.clearTimer(id);
    this.entries.set(id, { ...entry, state: 'unknown', reason });
    this.emit();
  }

  fail(message: ServerMessage): boolean {
    if (message.type !== 'error' || message.room_id !== this.roomID || !message.message_id) return false;
    const entry = this.entries.get(message.message_id);
    if (!entry) return false;
    this.clearTimer(message.message_id);
    this.entries.set(message.message_id, {
      ...entry, state: message.retryable === false ? 'rejected' : 'unknown', reason: message.content,
    });
    this.emit();
    return true;
  }

  confirm(message: ServerMessage): boolean {
    if ((message.type !== 'ack' && message.type !== 'danma') || !message.message_id ||
        !Number.isSafeInteger(message.id) || Number(message.id) <= 0 ||
        message.room_id !== this.roomID || message.user_id !== this.userID) return false;
    const entry = this.entries.get(message.message_id);
    if (!entry || entry.message.content !== message.content) return false;
    this.clearTimer(message.message_id);
    this.entries.delete(message.message_id);
    this.emit();
    return true;
  }

  pause(reason = '连接已断开，保存结果尚未确认；重连后可以重试原消息。') {
    let changed = false;
    for (const [id, entry] of this.entries) {
      this.clearTimer(id);
      if (entry.state === 'pending' || entry.state === 'accepted') {
        const detail = entry.state === 'accepted'
          ? '消息已接收，连接已断开，保存尚未确认；重连后可以重试原消息。'
          : reason;
        this.entries.set(id, { ...entry, state: 'unknown', reason: detail });
        changed = true;
      }
    }
    if (changed) this.emit();
  }

  remove(id: string) {
    const state = this.entries.get(id)?.state;
    if (state === 'pending' || state === 'accepted') return;
    this.clearTimer(id);
    if (this.entries.delete(id)) this.emit();
  }
}
