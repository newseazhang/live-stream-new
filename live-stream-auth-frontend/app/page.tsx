'use client';

import { useState } from 'react';
import { AuthForm } from '../components/auth-form';
import { RoomList } from '../components/room-list';
import { HotRoomList } from '../components/hot-room-list';
import { RoomStudio } from '../components/room-studio';
import { useAuth } from '../hooks/useAuth';

export default function HomePage() {
  const { session, identityVersion, checkSession, login, logout } = useAuth();
  const [leaving, setLeaving] = useState(false);
  const [logoutError, setLogoutError] = useState('');
  const [roomRevision, setRoomRevision] = useState(0);

  async function leave() {
    if (leaving) return;
    setLeaving(true);
    setLogoutError('');
    try {
      await logout();
    } catch (error) {
      setLogoutError(error instanceof Error ? error.message : '退出失败，请重试。');
    } finally {
      setLeaving(false);
    }
  }

  return (
    <main className="workspace">
      <header className="intro lobby-header">
        <div className="brand"><span className="brand-dot" aria-hidden="true" /> Live Stream</div>
        <h1>直播大厅</h1>
        <p>发现正在直播的房间，或者开启自己的直播。</p>
      </header>

      <div className="workspace-grid">
        <div className="room-column">
          {session.status === 'authenticated' && (
            <RoomStudio key={`${session.user.user_id}:${identityVersion}`} onChanged={() => setRoomRevision(value => value + 1)} onSessionExpired={checkSession} />
          )}
          <HotRoomList revision={roomRevision} />
          <RoomList revision={roomRevision} />
        </div>
        <section className="account-card" aria-label="账户管理">
          {session.status === 'checking' && <p className="checking" role="status">正在检查登录状态…</p>}

          {session.status === 'error' && (
            <>
              <h2>暂时无法读取账号</h2>
              <p className="feedback error" role="alert">{session.message}</p>
              <button className="primary-button" onClick={() => void checkSession()}>重试</button>
            </>
          )}

          {session.status === 'anonymous' && <AuthForm onLogin={login} />}

          {session.status === 'authenticated' && (
            <>
              <span className="status-badge"><span aria-hidden="true" /> 已登录</span>
              <h2 className="greeting">你好，{session.user.user_name}</h2>
              <dl className="profile">
                <div><dt>用户 ID</dt><dd>{session.user.user_id}</dd></div>
                <div><dt>邮箱</dt><dd>{session.user.email}</dd></div>
              </dl>
              {logoutError && <p className="feedback error" role="alert">{logoutError}</p>}
              <button className="secondary-button" disabled={leaving} onClick={() => void leave()}>
                {leaving ? '正在退出…' : '退出登录'}
              </button>
            </>
          )}
        </section>
      </div>
    </main>
  );
}
