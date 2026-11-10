'use client';

import { useState, type FormEvent } from 'react';
import { authApi } from '../lib/api';
import type { LoginInput } from '../lib/types';

type Mode = 'login' | 'register';

export function AuthForm({ onLogin }: { onLogin: (input: LoginInput) => Promise<void> }) {
  const [mode, setMode] = useState<Mode>('login');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const registering = mode === 'register';

  function switchMode(next: Mode) {
    setMode(next);
    setPassword('');
    setNotice('');
    setError('');
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    setPending(true);
    setError('');
    setNotice('');

    try {
      const input = { email: email.trim(), password };
      if (registering) {
        const user = await authApi.register({ ...input, username: username.trim() });
        setEmail(user.email);
        switchMode('login');
        setNotice('注册成功，请用这个邮箱和密码登录。');
      } else {
        await onLogin(input);
      }
    } catch (error) {
      setError(error instanceof Error ? error.message : '提交失败，请重试。');
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <div className="mode-switch" aria-label="账户操作">
        <button type="button" disabled={pending} aria-pressed={!registering} onClick={() => switchMode('login')}>
          登录
        </button>
        <button type="button" disabled={pending} aria-pressed={registering} onClick={() => switchMode('register')}>
          注册
        </button>
      </div>
      <div className="card-heading">
        <h2>{registering ? '创建账号' : '登录账号'}</h2>
        <p>{registering ? '填写用户名、邮箱和密码。' : '使用你注册时的邮箱和密码。'}</p>
      </div>
      {notice && <p className="feedback success" role="status">{notice}</p>}
      {error && <p className="feedback error" role="alert">{error}</p>}
      <form onSubmit={submit} aria-busy={pending}>
        <fieldset disabled={pending}>
          {registering && (
            <label htmlFor="username">
              用户名
              <input id="username" name="username" autoComplete="username" required value={username} onChange={event => setUsername(event.target.value)} placeholder="2–32 个字符" />
            </label>
          )}
          <label htmlFor="email">
            邮箱
            <input id="email" name="email" type="email" autoComplete="username" required value={email} onChange={event => setEmail(event.target.value)} placeholder="you@example.com" />
          </label>
          <label htmlFor="password">
            密码
            <input id="password" name="password" type="password" autoComplete={registering ? 'new-password' : 'current-password'} required value={password} onChange={event => setPassword(event.target.value)} placeholder={registering ? '15–128 个字符' : '输入密码'} />
          </label>
          <button className="primary-button" type="submit">
            {pending ? '提交中…' : registering ? '创建账号' : '登录'}
          </button>
        </fieldset>
      </form>
    </>
  );
}
