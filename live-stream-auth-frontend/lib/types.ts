export interface User {
  user_id: number;
  user_name: string;
  email: string;
}

export interface LoginInput {
  email: string;
  password: string;
}

export interface RegisterInput extends LoginInput {
  username: string;
}

export interface Room {
  room_id: number;
  owner_id: number;
  room_title: string;
  status: 0 | 1;
  start_at: string | null;
  like_count: number;
}

export interface HotRoom {
  room_id: number;
  heat_score: number;
}
