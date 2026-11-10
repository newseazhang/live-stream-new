export interface ServerMessage {
  id?: number;
  type: 'danma' | 'accepted' | 'ack' | 'system' | 'error' | 'room_state';
  message_id?: string;
  room_id: number;
  user_id?: number;
  content: string;
  sent_at: string;
  like_count?: number;
  online_count?: number;
  code?: string;
  retryable?: boolean;
}

export interface ClientDanma {
  readonly type: 'danma';
  readonly message_id: string;
  readonly content: string;
}

export function roomWebSocketURL(roomID: number): string {
  if (!Number.isSafeInteger(roomID) || roomID <= 0) {
    throw new Error('房间 ID 无效。');
  }
  const url = new URL(`/api/v1/rooms/${roomID}/ws`, window.location.origin);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

export function parseServerMessage(raw: unknown): ServerMessage {
  if (typeof raw !== 'string') throw new Error('收到非文本消息。');
  return validateServerMessage(JSON.parse(raw));
}

export function validateServerMessage(value: unknown): ServerMessage {
  if (!value || typeof value !== 'object') throw new Error('消息格式不正确。');
  const message = value as Partial<ServerMessage>;
  if (
    !['danma', 'accepted', 'ack', 'system', 'error', 'room_state'].includes(message.type ?? '') ||
    !Number.isSafeInteger(message.room_id) || Number(message.room_id) <= 0 ||
    (message.id !== undefined && (!Number.isSafeInteger(message.id) || message.id <= 0)) ||
    (message.message_id !== undefined && (typeof message.message_id !== 'string' || !message.message_id)) ||
    (message.code !== undefined && (typeof message.code !== 'string' || !message.code)) ||
    (message.retryable !== undefined && typeof message.retryable !== 'boolean') ||
    (typeof message.content !== 'string' && !(message.type === 'room_state' && message.content === undefined)) ||
    typeof message.sent_at !== 'string' || !Number.isFinite(Date.parse(message.sent_at)) ||
    (message.type === 'room_state' && (
      (message.like_count === undefined && message.online_count === undefined) ||
      (message.like_count !== undefined && (!Number.isSafeInteger(message.like_count) || message.like_count < 0)) ||
      (message.online_count !== undefined && (!Number.isSafeInteger(message.online_count) || message.online_count < 0))
    )) ||
    (message.type === 'ack' && message.id === undefined) ||
    (message.type === 'accepted' && message.id !== undefined) ||
    ((message.type === 'danma' || message.type === 'ack' || message.type === 'accepted') && (
      typeof message.message_id !== 'string' || !message.message_id ||
      !Number.isSafeInteger(message.user_id) || Number(message.user_id) <= 0
    ))
  ) {
    throw new Error('服务端消息格式不正确。');
  }
  return { ...message, content: message.content ?? '' } as ServerMessage;
}

export function createDanma(content: string): ClientDanma {
  const trimmed = content.trim();
  const length = Array.from(trimmed).length;
  if (length < 1 || length > 200) throw new Error('弹幕需要 1–200 个字符。');
  if (typeof globalThis.crypto?.randomUUID !== 'function') {
    throw new Error('当前环境无法生成 UUID，请使用 localhost、127.0.0.1 或 HTTPS。');
  }
  return { type: 'danma' as const, message_id: crypto.randomUUID(), content: trimmed };
}
