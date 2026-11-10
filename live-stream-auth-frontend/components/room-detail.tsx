'use client';

import { useEffect, useState } from 'react';
import type { MouseEvent } from 'react';
import { useAuth } from '../hooks/useAuth';
import { roomsApi } from '../lib/api';
import { leaveCurrentRoom } from '../lib/room-lifecycle';
import type { Room } from '../lib/types';
import { DanmaPanel } from './danma-panel';

export function RoomDetail({ roomID }: { roomID: string }) {
  const { session, identityVersion, checkSession, logout } = useAuth();
  const [room, setRoom] = useState<Room | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  const [loggingOut, setLoggingOut] = useState(false);
  const [authError, setAuthError] = useState('');

  function returnToLobby(event: MouseEvent<HTMLAnchorElement>) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    leaveCurrentRoom();
    // 明确结束当前房间，再完整跳转；不依赖路由缓存是否卸载组件。
    window.location.assign(event.currentTarget.href);
  }

  async function handleLogout() {
    setLoggingOut(true);
    setAuthError('');
    try {
      await logout();
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : '退出失败。');
    } finally {
      setLoggingOut(false);
    }
  }

  useEffect(() => {
    const request = new AbortController();
    const id = Number(roomID);
    setRoom(null);
    setError('');
    setLoading(true);
    if (!/^\d+$/.test(roomID) || !Number.isSafeInteger(id) || id <= 0) {
      setError('房间 ID 无效。');
      setLoading(false);
      return;
    }
    roomsApi.detail(id, request.signal).then(result => {
      if (!request.signal.aborted) setRoom(result);
    }).catch(error => {
      if (!request.signal.aborted) setError(error instanceof Error ? error.message : '读取房间详情失败。');
    }).finally(() => {
      if (!request.signal.aborted) setLoading(false);
    });
    return () => request.abort();
  }, [roomID, revision]);

  return (
    <main className="room-page">
      <a className="back-link" href="/" onClick={returnToLobby}>← 返回直播大厅</a>
      <section className="panel detail-panel">
        <div className="section-heading">
          <h1>{room?.room_title ?? '房间详情'}</h1>
          <button className="secondary-button compact" disabled={loading} onClick={() => setRevision(value => value + 1)}>刷新状态</button>
        </div>
        {loading && <p className="muted" role="status">正在读取房间…</p>}
        {error && <p className="feedback error" role="alert">{error}</p>}
        {room && (
          <>
            <span className={`room-status ${room.status === 1 ? 'live' : ''}`}>{room.status === 1 ? '直播中' : '待开播'}</span>
            <dl className="profile detail-profile">
              <div><dt>房间 ID</dt><dd>{room.room_id}</dd></div>
              <div><dt>房主 ID</dt><dd>{room.owner_id}</dd></div>
              <div><dt>开播时间</dt><dd>{room.start_at ? new Date(room.start_at).toLocaleString('zh-CN') : '尚未开播'}</dd></div>
            </dl>
            <div className="stage-placeholder"><span className="stage-symbol" aria-hidden="true">◉</span><h2>{room.status === 1 ? '房间已开播' : '等待房主开播'}</h2></div>
          </>
        )}
      </section>
      <section className="room-account" aria-label="房间登录状态">
        <p className="muted">
          {session.status === 'checking' && '正在检查登录状态…'}
          {session.status === 'anonymous' && '当前以游客身份观看。'}
          {session.status === 'authenticated' && `你好，${session.user.user_name}`}
          {session.status === 'error' && session.message}
        </p>
        <div className="danma-actions">
          <button className="secondary-button compact" disabled={session.status === 'checking' || loggingOut} onClick={() => void checkSession()}>刷新登录状态</button>
          {session.status === 'authenticated'
            ? <button className="secondary-button compact" disabled={loggingOut} onClick={() => void handleLogout()}>{loggingOut ? '正在退出…' : '退出登录'}</button>
            : <a className="back-link" href="/">前往大厅登录</a>}
        </div>
        {authError && <p className="feedback error" role="alert">{authError}</p>}
      </section>
      {room && room.room_id === Number(roomID) && (session.status === 'authenticated' || session.status === 'anonymous') && (
        <DanmaPanel
          key={`${room.room_id}:${room.status}:${identityVersion}:${session.status === 'authenticated' ? session.user.user_id : 'guest'}`}
          roomID={room.room_id}
          live={room.status === 1}
          initialLikeCount={room.like_count}
          userID={session.status === 'authenticated' ? session.user.user_id : null}
        />
      )}
    </main>
  );
}
