'use client';

import { useEffect, useState } from 'react';
import { roomsApi } from '../lib/api';
import type { Room } from '../lib/types';

export function RoomList({ revision }: { revision: number }) {
  const [rooms, setRooms] = useState<Room[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const request = new AbortController();
    setLoading(true);
    setError('');
    roomsApi.list(request.signal).then(result => {
      if (!request.signal.aborted) setRooms(result);
    }).catch(error => {
      if (!request.signal.aborted) setError(error instanceof Error ? error.message : '读取直播列表失败。');
    }).finally(() => {
      if (!request.signal.aborted) setLoading(false);
    });
    return () => request.abort();
  }, [revision, reload]);

  return (
    <section className="panel live-list" aria-labelledby="live-heading">
      <div className="section-heading">
        <div><h2 id="live-heading">正在直播</h2><p className="muted">按最近开播时间排序，最多展示 20 个房间</p></div>
        <button className="secondary-button compact" disabled={loading} onClick={() => setReload(value => value + 1)}>
          {loading ? '刷新中…' : '刷新列表'}
        </button>
      </div>
      {error && <p className="feedback error" role="alert">{error}</p>}
      {!loading && !error && rooms.length === 0 && (
        <div className="empty-state"><h3>还没有人开播</h3><p>创建房间，开启第一场直播。</p></div>
      )}
      <div className="room-grid" aria-busy={loading}>
        {rooms.map(room => (
          <a key={room.room_id} href={`/rooms/${room.room_id}`} className="room-card">
            <div className="room-cover"><span className="room-status live">直播中</span><span className="room-number">#{room.room_id}</span></div>
            <div className="room-card-body"><h3>{room.room_title}</h3><p className="muted">房主 #{room.owner_id}<span>进入房间 →</span></p></div>
          </a>
        ))}
      </div>
    </section>
  );
}
