'use client';

import { useEffect, useState } from 'react';
import { roomsApi } from '../lib/api';
import type { HotRoom } from '../lib/types';

export function HotRoomList({ revision }: { revision: number }) {
  const [rooms, setRooms] = useState<HotRoom[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const request = new AbortController();
    setLoading(true);
    setError('');
    roomsApi.hot(request.signal).then(result => {
      if (!request.signal.aborted) setRooms(result);
    }).catch(error => {
      if (!request.signal.aborted) setError(error instanceof Error ? error.message : '读取热度榜失败。');
    }).finally(() => {
      if (!request.signal.aborted) setLoading(false);
    });
    return () => request.abort();
  }, [revision, reload]);

  return (
    <section className="panel hot-list" aria-labelledby="hot-heading">
      <div className="section-heading">
        <div><h2 id="hot-heading">热门直播</h2><p className="muted">按在线情况和近期互动热度排序</p></div>
        <button className="secondary-button compact" disabled={loading} onClick={() => setReload(value => value + 1)}>
          {loading ? '刷新中…' : '刷新热度榜'}
        </button>
      </div>
      {error && <p className="feedback error" role="alert">{error}</p>}
      {loading && <p className="muted" role="status">正在读取热度榜…</p>}
      {!loading && !error && rooms.length === 0 && (
        <div className="empty-state"><h3>暂无热度排名</h3><p>开播后稍等片刻，点击刷新查看。</p></div>
      )}
      {!loading && !error && rooms.length > 0 && (
        <ol className="hot-rooms">
          {rooms.map((room, index) => (
            <li key={room.room_id}>
              <a href={`/rooms/${room.room_id}`} className="hot-room">
                <span className="hot-position" aria-label={`第 ${index + 1} 名`}>{index + 1}</span>
                <span className="hot-room-name">房间 #{room.room_id}<small>进入直播间 →</small></span>
                <span className="hot-score"><strong>{room.heat_score.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}</strong><small>热度</small></span>
              </a>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
