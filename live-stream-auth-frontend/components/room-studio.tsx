'use client';

import { useEffect, useState, type FormEvent } from 'react';
import { ApiError, roomsApi } from '../lib/api';
import type { Room } from '../lib/types';

interface Props {
  onChanged: () => void;
  onSessionExpired: () => Promise<void>;
}

export function RoomStudio({ onChanged, onSessionExpired }: Props) {
  const [room, setRoom] = useState<Room | null>(null);
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [revision, setRevision] = useState(0);
  const [title, setTitle] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  useEffect(() => {
    const request = new AbortController();
    setLoading(true);
    setError('');
    roomsApi.mine(request.signal).then(result => {
      if (!request.signal.aborted) {
        setRoom(result);
        setLoaded(true);
      }
    }).catch(async error => {
      if (request.signal.aborted) return;
      if (error instanceof ApiError && error.status === 401) {
        await onSessionExpired();
        return;
      }
      setError(error instanceof Error ? error.message : '查询我的房间失败。');
    }).finally(() => {
      if (!request.signal.aborted) setLoading(false);
    });
    return () => request.abort();
  }, [revision, onSessionExpired]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || loading) return;
    const normalizedTitle = title.trim();
    if (!room && (!normalizedTitle || Array.from(normalizedTitle).length > 120)) {
      setError('房间标题须为 1–120 个字符。');
      return;
    }
    setPending(true);
    setError('');
    setNotice('');
    try {
      if (room) {
        await roomsApi.start(room.room_id);
        setRoom(await roomsApi.mine());
        setNotice('开播成功，房间已进入公开列表。');
      } else {
        setRoom(await roomsApi.create(normalizedTitle));
        setNotice('房间已创建，可以开播了。');
      }
      onChanged();
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        await onSessionExpired();
      } else if (error instanceof ApiError && error.status === 409) {
        setRevision(value => value + 1);
        setNotice('房间状态已有变化，正在重新读取。');
        onChanged();
      } else {
        setError(error instanceof Error ? error.message : '操作失败，请刷新房间状态后重试。');
      }
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="panel studio" aria-labelledby="studio-heading">
      <div className="section-heading">
        <h2 id="studio-heading">我的房间</h2>
        <button className="text-button" disabled={loading || pending} onClick={() => {
          setNotice('');
          setRevision(value => value + 1);
        }}>刷新状态</button>
      </div>
      {loading && <p className="muted" role="status">正在读取房间…</p>}
      {error && <p className="feedback error" role="alert">{error}</p>}
      {notice && <p className="feedback success" role="status">{notice}</p>}
      {!loading && (
        <form onSubmit={submit}>
          {room ? (
            <>
              <span className={`room-status ${room.status === 1 ? 'live' : ''}`}>
                {room.status === 1 ? '直播中' : '待开播'}
              </span>
              <h3 className="room-title">{room.room_title}</h3>
              <p className="muted">房间 #{room.room_id}</p>
              <div className="action-row">
                {room.status === 0 && <button className="primary-button" disabled={pending} type="submit">
                  {pending ? '正在开播…' : '开始直播'}
                </button>}
                <a className="secondary-button button-link" href={`/rooms/${room.room_id}`}>进入房间</a>
              </div>
            </>
          ) : loaded && (
            <>
              <p className="muted">创建你的直播间，开播后大家就能在大厅找到你。</p>
              <label htmlFor="room-title">房间标题
                <input id="room-title" value={title} required disabled={pending}
                  onChange={event => setTitle(event.target.value)} placeholder="给直播间起个名字" />
              </label>
              <button className="primary-button create-room" type="submit" disabled={pending}>
                {pending ? '正在创建…' : '创建房间'}
              </button>
            </>
          )}
        </form>
      )}
    </section>
  );
}
