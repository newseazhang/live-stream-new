import type { HotRoom, LoginInput, RegisterInput, Room, User } from './types';
import { parseMessageHistory } from './message-history';

const authPath = '/api/v1/auth';

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request(path: string, init: RequestInit = {}): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(path, {
      ...init,
      credentials: 'same-origin',
      cache: 'no-store',
    });
  } catch (error) {
    if (init.signal?.aborted) throw error;
    throw new ApiError(0, '连接失败，请确认后端服务正在运行。');
  }

  if (!response.ok) {
    const body = await response.json().catch(() => null);
    const message = typeof body?.message === 'string' && body.message.trim()
      ? body.message
      : `请求失败（HTTP ${response.status}）`;
    throw new ApiError(response.status, message);
  }

  return response;
}

async function requestUser(path: string, init?: RequestInit): Promise<User> {
  const response = await request(`${authPath}${path}`, init);
  const user: Partial<User> | null = await response.json().catch(() => null);

  if (
    !user || typeof user.user_id !== 'number' ||
    !Number.isInteger(user.user_id) || user.user_id <= 0 ||
    typeof user.user_name !== 'string' || typeof user.email !== 'string'
  ) {
    throw new ApiError(response.status, '后端返回的用户资料格式不正确。');
  }

  return { user_id: user.user_id, user_name: user.user_name, email: user.email };
}

function jsonPost(input: LoginInput | RegisterInput | { title: string }): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  };
}

export const authApi = {
  register: (input: RegisterInput) => requestUser('/register', jsonPost(input)),
  login: (input: LoginInput) => requestUser('/login', jsonPost(input)),
  me: (signal?: AbortSignal) => requestUser('/me', { signal }),
  logout: async () => {
    await request(`${authPath}/logout`, { method: 'POST' });
  },
};

const roomsPath = '/api/v1/rooms';

function parseRoom(value: unknown): Room {
  const room = value as Partial<Room> | null;
  if (
    !room || !Number.isSafeInteger(room.room_id) || Number(room.room_id) <= 0 ||
    !Number.isSafeInteger(room.owner_id) || Number(room.owner_id) <= 0 ||
    typeof room.room_title !== 'string' || (room.status !== 0 && room.status !== 1) ||
    (room.start_at !== null && typeof room.start_at !== 'string') ||
    !Number.isSafeInteger(room.like_count) || Number(room.like_count) < 0
  ) {
    throw new ApiError(200, '后端返回的房间资料格式不正确。');
  }
  return room as Room;
}

async function roomData(path: string, signal?: AbortSignal): Promise<unknown> {
  const response = await request(path, { signal });
  const body = await response.json().catch(() => null);
  if (!body || !Object.hasOwn(body, 'data')) {
    throw new ApiError(response.status, '后端返回的房间资料缺少 data 字段。');
  }
  return body.data;
}

export const roomsApi = {
  hot: async (signal?: AbortSignal): Promise<HotRoom[]> => {
    const data = await roomData(`${roomsPath}/hot?limit=10`, signal);
    if (!Array.isArray(data)) throw new ApiError(200, '后端返回的热度榜格式不正确。');
    const seen = new Set<number>();
    return data.map(value => {
      const room = value as Partial<HotRoom> | null;
      if (!room || !Number.isSafeInteger(room.room_id) || Number(room.room_id) <= 0 ||
          typeof room.heat_score !== 'number' || !Number.isFinite(room.heat_score) || room.heat_score < 0 ||
          seen.has(Number(room.room_id))) {
        throw new ApiError(200, '后端返回的热度榜格式不正确。');
      }
      seen.add(Number(room.room_id));
      return { room_id: Number(room.room_id), heat_score: room.heat_score };
    });
  },
  messages: async (roomID: number, beforeID?: number, signal?: AbortSignal) => {
    const query = new URLSearchParams({ limit: '20' });
    if (beforeID !== undefined) query.set('before_id', String(beforeID));
    const response = await request(`${roomsPath}/${roomID}/messages?${query}`, { signal });
    return parseMessageHistory(await response.json(), roomID, beforeID);
  },
  list: async (signal?: AbortSignal): Promise<Room[]> => {
    const data = await roomData(roomsPath, signal);
    if (!Array.isArray(data)) throw new ApiError(200, '后端返回的房间列表格式不正确。');
    return data.map(parseRoom);
  },
  mine: async (signal?: AbortSignal): Promise<Room | null> => {
    const data = await roomData(`${roomsPath}/mine`, signal);
    return data === null ? null : parseRoom(data);
  },
  detail: async (roomID: number, signal?: AbortSignal): Promise<Room> => {
    return parseRoom(await roomData(`${roomsPath}/${roomID}`, signal));
  },
  create: async (title: string): Promise<Room> => {
    const response = await request(roomsPath, jsonPost({ title }));
    return parseRoom(await response.json());
  },
  start: async (roomID: number): Promise<void> => {
    await request(`${roomsPath}/${roomID}/start`, { method: 'POST' });
  },
};
