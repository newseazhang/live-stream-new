import { validateServerMessage } from './websocket';
import type { ServerMessage } from './websocket';

export interface DanmaMessage extends ServerMessage {
  type: 'danma';
  message_id: string;
  user_id: number;
}

export interface HistoryMessage extends DanmaMessage {
  id: number;
}

export interface MessageHistoryPage {
  data: HistoryMessage[];
  has_more: boolean;
  next_before_id: number;
}

export function parseMessageHistory(value: unknown, roomID: number, beforeID?: number): MessageHistoryPage {
  const page = value as Partial<MessageHistoryPage> | null;
  if (!page || !Array.isArray(page.data) || page.data.length > 20 ||
      typeof page.has_more !== 'boolean' || !Number.isSafeInteger(page.next_before_id) || Number(page.next_before_id) < 0) {
    throw new Error('历史消息响应格式不正确。');
  }
  let previousID = 0;
  const uuids = new Set<string>();
  const data = page.data.map(value => {
    const message = validateServerMessage(value);
    if (message.type !== 'danma' || message.room_id !== roomID || message.id === undefined ||
        message.id <= previousID || (beforeID !== undefined && message.id >= beforeID) || uuids.has(message.message_id!)) {
      throw new Error('历史消息的房间、排序或游标不正确。');
    }
    previousID = message.id;
    uuids.add(message.message_id!);
    return message as HistoryMessage;
  });
  if ((page.has_more && (!data.length || page.next_before_id !== data[0].id)) ||
      (!page.has_more && page.next_before_id !== 0)) {
    throw new Error('历史消息的下一页游标不正确。');
  }
  return { data, has_more: page.has_more, next_before_id: page.next_before_id! };
}

// HTTP、ACK 和新版实时广播均携带数据库 id；兼容旧广播只有 UUID。
export function mergeDanma(current: DanmaMessage[], incoming: DanmaMessage[]): DanmaMessage[] {
  const messages = new Map(current.map(message => [message.message_id, message]));
  for (const message of incoming) {
    const previous = messages.get(message.message_id);
    messages.set(message.message_id, { ...previous, ...message, id: message.id ?? previous?.id });
  }
  return [...messages.values()].sort((a, b) => {
    if (a.id !== undefined && b.id !== undefined) return a.id - b.id;
    if (a.id !== undefined) return -1;
    if (b.id !== undefined) return 1;
    return 0;
  });
}
