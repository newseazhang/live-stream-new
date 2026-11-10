'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, authApi } from '../lib/api';
import { createAuthSync } from '../lib/auth-sync';
import type { LoginInput, User } from '../lib/types';

type Session =
  | { status: 'checking' }
  | { status: 'anonymous' }
  | { status: 'authenticated'; user: User }
  | { status: 'error'; message: string };

export function useAuth() {
  const [session, setSession] = useState<Session>({ status: 'checking' });
  const [identityVersion, setIdentityVersion] = useState(0);
  const mountedRef = useRef(false);
  const requestRef = useRef<AbortController | null>(null);
  const sequenceRef = useRef(0);
  const syncRef = useRef<ReturnType<typeof createAuthSync> | null>(null);

  const cancelRead = useCallback(() => {
    sequenceRef.current += 1;
    requestRef.current?.abort();
    requestRef.current = null;
  }, []);

  // 后台检查不切换 checking，身份未变时保留历史列表与当前 WS。
  const readSession = useCallback(async (signal?: AbortSignal) => {
    if (!mountedRef.current || signal?.aborted) return;
    cancelRead();
    const sequence = sequenceRef.current;
    const request = new AbortController();
    requestRef.current = request;
    const abort = () => request.abort();
    signal?.addEventListener('abort', abort, { once: true });
    const current = () => mountedRef.current && !request.signal.aborted && sequenceRef.current === sequence;
    try {
      const user = await authApi.me(request.signal);
      if (current()) setSession({ status: 'authenticated', user });
    } catch (error) {
      if (!current()) return;
      if (error instanceof ApiError && error.status === 401) {
        setSession({ status: 'anonymous' });
        return;
      }
      setSession({
        status: 'error',
        message: error instanceof Error ? error.message : '无法读取登录状态。',
      });
    } finally {
      signal?.removeEventListener('abort', abort);
      if (requestRef.current === request) requestRef.current = null;
    }
  }, [cancelRead]);

  // 显式身份变更时立即卸载旧身份对应的 WS，并强制重建同账号的新 Session。
  const checkSession = useCallback(async (signal?: AbortSignal) => {
    if (!mountedRef.current || signal?.aborted) return;
    setSession({ status: 'checking' });
    setIdentityVersion(current => current + 1);
    await readSession(signal);
  }, [readSession]);

  useEffect(() => {
    mountedRef.current = true;
    const sync = createAuthSync(() => { void checkSession(); });
    syncRef.current = sync;
    void readSession();
    let lastFocusCheck = -Infinity;
    const onFocus = () => {
      if (document.visibilityState !== 'visible') return;
      const now = performance.now();
      if (now - lastFocusCheck < 100) return;
      lastFocusCheck = now;
      if (!sync.checkForChanges()) void readSession();
    };
    window.addEventListener('focus', onFocus);
    document.addEventListener('visibilitychange', onFocus);
    return () => {
      mountedRef.current = false;
      cancelRead();
      sync.close();
      if (syncRef.current === sync) syncRef.current = null;
      window.removeEventListener('focus', onFocus);
      document.removeEventListener('visibilitychange', onFocus);
    };
  }, [checkSession, readSession, cancelRead]);

  async function login(input: LoginInput) {
    const sync = syncRef.current;
    cancelRead();
    await authApi.login(input);
    sync?.notify();
    await checkSession();
  }

  async function logout() {
    const sync = syncRef.current;
    cancelRead();
    await authApi.logout();
    sync?.notify();
    await checkSession();
  }

  return { session, identityVersion, checkSession, login, logout };
}
