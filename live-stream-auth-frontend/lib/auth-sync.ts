const CHANNEL_NAME = 'live-stream-auth';
const STORAGE_KEY = 'live-stream:auth-changed';

type AuthChange = { type: 'session-changed'; nonce: string };

function parseChange(value: unknown): AuthChange | null {
  if (!value || typeof value !== 'object') return null;
  const change = value as Partial<AuthChange>;
  return change.type === 'session-changed' && typeof change.nonce === 'string' && change.nonce.length <= 128
    ? change as AuthChange : null;
}

// 消息只是重新查询 /me 的提示，不携带 Cookie 或用户资料。
export function createAuthSync(onChange: () => void) {
  const seen = new Set<string>();
  let closed = false;
  let channel: BroadcastChannel | null = null;

  function remember(nonce: string) {
    seen.add(nonce);
    if (seen.size > 64) seen.delete(seen.values().next().value!);
  }

  function readStorage(): AuthChange | null {
    try {
      return parseChange(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? 'null'));
    } catch {
      return null;
    }
  }

  function receive(value: unknown): boolean {
    const change = parseChange(value);
    if (closed || !change || seen.has(change.nonce)) return false;
    remember(change.nonce);
    onChange();
    return true;
  }

  const initial = readStorage();
  if (initial) remember(initial.nonce);

  try {
    if (typeof BroadcastChannel !== 'undefined') {
      channel = new BroadcastChannel(CHANNEL_NAME);
      channel.onmessage = event => { receive(event.data); };
    }
  } catch {
    channel = null;
  }

  const onStorage = (event: StorageEvent) => {
    if (event.key !== STORAGE_KEY || !event.newValue) return;
    try { receive(JSON.parse(event.newValue)); } catch { /* 忽略其他脚本写入的非 JSON。 */ }
  };
  window.addEventListener('storage', onStorage);

  return {
    notify() {
      const change: AuthChange = {
        type: 'session-changed',
        nonce: typeof globalThis.crypto?.randomUUID === 'function'
          ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`,
      };
      remember(change.nonce);
      // 两条通道使用相同 nonce，接收者只处理一次。
      try { channel?.postMessage(change); } catch { /* 继续尝试 storage。 */ }
      try { window.localStorage.setItem(STORAGE_KEY, JSON.stringify(change)); } catch { /* 恢复焦点时重查。 */ }
    },
    checkForChanges() { return receive(readStorage()); },
    close() {
      closed = true;
      window.removeEventListener('storage', onStorage);
      channel?.close();
    },
  };
}
