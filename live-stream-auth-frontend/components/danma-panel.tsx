'use client';

import { useId, useLayoutEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { useDanma } from '../hooks/use-danma';
import type { ConnectionStatus } from '../hooks/use-danma';

const statusText: Record<ConnectionStatus, string> = {
  idle: '未开播', connecting: '正在连接', open: '已连接', closed: '已断开', error: '连接异常',
};

export function DanmaPanel({ roomID, userID, live = true, initialLikeCount = 0 }: { roomID: number; userID: number | null; live?: boolean; initialLikeCount?: number }) {
  const {
    status, messages, notice, error, historyError, historyLoading, historyLoaded, hasMore, historyVersion,
    send, reconnect, loadMore,
    outgoing, retry, removeOutgoing,
    likeCount, onlineCount, likeCooling, likeSyncing, likeError, like, refreshLikes,
  } = useDanma(roomID, userID, live, initialLikeCount);
  const [draft, setDraft] = useState('');
  const listRef = useRef<HTMLDivElement>(null);
  const followLatest = useRef(true);
  const anchorRef = useRef<{ id: string; top: number; version: number } | null>(null);
  const loadingEarlierRef = useRef(false);
  const inputID = useId();
  const length = Array.from(draft.trim()).length;
  const canSend = live && userID !== null && status === 'open';

  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    if (messages.length === 0) {
      anchorRef.current = null;
      followLatest.current = true;
      return;
    }
    const anchor = anchorRef.current;
    if (anchor && historyVersion >= anchor.version) {
      const item = [...list.querySelectorAll<HTMLElement>('[data-message-id]')].find(item => item.dataset.messageId === anchor.id);
      if (item) list.scrollTop += item.getBoundingClientRect().top - list.getBoundingClientRect().top - anchor.top;
      anchorRef.current = null;
    } else if (!anchor && followLatest.current) {
      list.scrollTop = list.scrollHeight;
    }
  }, [messages, historyVersion]);

  async function loadEarlier() {
    if (loadingEarlierRef.current) return;
    loadingEarlierRef.current = true;
    const list = listRef.current;
    if (list) {
      const top = list.getBoundingClientRect().top;
      const first = [...list.querySelectorAll<HTMLElement>('[data-message-id]')].find(item => item.getBoundingClientRect().bottom > top);
      if (first?.dataset.messageId) {
        followLatest.current = false;
        anchorRef.current = { id: first.dataset.messageId, top: first.getBoundingClientRect().top - top, version: historyVersion + 1 };
      }
    }
    if (!await loadMore()) anchorRef.current = null;
    loadingEarlierRef.current = false;
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (send(draft)) {
      setDraft('');
      anchorRef.current = null;
      followLatest.current = true;
    }
  }

  return (
    <section className="panel danma-panel" aria-label="弹幕与历史">
      <div className="section-heading">
        <h2>弹幕与历史</h2>
        <div className="danma-actions">
          <span className={`room-status ${status === 'open' ? 'live' : ''}`} role="status">{statusText[status]}</span>
          {live && <button className="secondary-button compact" disabled={status === 'connecting'} onClick={reconnect}>重新连接</button>}
        </div>
      </div>
      <p className="muted">{userID === null ? '游客可以观看，登录后可发送弹幕。' : `当前账号 ID：${userID}`}</p>
      <section className="like-bar" aria-label="房间点赞">
        <div className="like-counter">
          <span className="muted">累计点赞</span>
          <strong aria-label="累计点赞数" role="status" aria-live="polite">{likeCount.toLocaleString('zh-CN')}</strong>
        </div>
        <div className="like-counter">
          <span className="muted">在线连接</span>
          <strong aria-label="在线连接数" role="status" aria-live="polite">{status === 'open' && onlineCount !== null ? onlineCount.toLocaleString('zh-CN') : '—'}</strong>
        </div>
        <div className="danma-actions">
          <button className="like-button" aria-label="点赞" disabled={!canSend || likeCooling} onClick={like}>
            <span aria-hidden="true">♥</span> {likeCooling ? '稍等一下' : '点赞'}
          </button>
          <button className="secondary-button compact" disabled={likeSyncing} onClick={() => void refreshLikes()}>
            {likeSyncing ? '正在刷新…' : '刷新点赞数'}
          </button>
        </div>
        <p className="muted like-hint">{!live ? '房间未开播，可浏览历史。' : userID === null ? '登录后可以为房间点赞。' : '喜欢这个房间，就为它点个赞。'}</p>
      </section>
      {likeError && <p className="feedback error" role="alert">{likeError}</p>}
      {error && <p className="feedback error" role="alert">{error}</p>}
      <div className="history-toolbar">
        <button className="secondary-button compact" disabled={historyLoading || (historyLoaded && !hasMore && !historyError)} onClick={() => void loadEarlier()}>
          {historyLoading ? '正在加载历史…' : historyError ? '重试加载历史' : hasMore || !historyLoaded ? '加载更早消息' : '没有更早的消息'}
        </button>
        <span className="muted">已显示 {messages.length} 条</span>
      </div>
      {historyError && <p className="feedback error" role="alert">历史加载失败：{historyError}</p>}
      <div
        className="danma-list"
        ref={listRef}
        role="log"
        aria-label="弹幕消息"
        aria-live="polite"
        aria-busy={historyLoading}
        tabIndex={0}
        onScroll={() => {
          const list = listRef.current;
          if (list) followLatest.current = list.scrollHeight - list.scrollTop - list.clientHeight < 48;
        }}
      >
        {notice && <p className="muted">{notice}</p>}
        {messages.length === 0 && !historyLoading && !historyError && <p className="muted">暂无弹幕。</p>}
        {messages.map(message => (
          <div className="danma-message danma" key={message.message_id} data-message-id={message.message_id}>
            {message.type === 'danma' && (
              <span className="danma-author">{message.user_id === userID ? '我' : `用户 ${message.user_id}`}：</span>
            )}
            <span>{message.content}</span>
            <time dateTime={message.sent_at}>{new Date(message.sent_at).toLocaleTimeString('zh-CN')}</time>
            {message.user_id === userID && message.id !== undefined && <span className="danma-saved">已保存</span>}
          </div>
        ))}
      </div>
      {outgoing.length > 0 && (
        <section className="danma-outbox" aria-label="待处理弹幕">
          <h3>待处理弹幕</h3>
          <p className="muted">已接收表示正在等待保存确认；超时后可重试原消息。刷新页面、离开或身份更新会清空本页待处理记录。</p>
          <ul>
            {outgoing.map(entry => (
              <li key={entry.message.message_id} data-outgoing-id={entry.message.message_id}>
                <p className="outgoing-content">{entry.message.content}</p>
                <span className="outgoing-state" role="status">
                  {entry.state === 'pending' ? '等待接收确认' : entry.state === 'accepted' ? '已接收，等待保存' : entry.state === 'unknown' ? '保存未确认' : '未接受'}
                </span>
                <span className="muted"> 已尝试 {entry.attempts} 次</span>
                {entry.reason && <p className="muted">{entry.reason}</p>}
                <div className="danma-actions">
                  {entry.state !== 'rejected' && <button className="secondary-button compact" disabled={!canSend || entry.state !== 'unknown'} onClick={() => retry(entry.message.message_id)}>重试原消息</button>}
                  {(entry.state === 'unknown' || entry.state === 'rejected') && <button className="text-button" onClick={() => removeOutgoing(entry.message.message_id)}>移除本页记录</button>}
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
      <form className="danma-form" onSubmit={submit}>
        <label htmlFor={inputID}>弹幕内容</label>
        <div className="danma-input-row">
          <input
            id={inputID}
            value={draft}
            onChange={event => setDraft(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter' && (event.nativeEvent.isComposing || event.keyCode === 229)) event.preventDefault();
            }}
            disabled={!canSend}
            placeholder={!live ? '房间未开播，可浏览历史' : userID === null ? '登录后发送弹幕' : '说点什么，按回车发送'}
            autoComplete="off"
            aria-describedby={`${inputID}-count`}
          />
          <button className="primary-button compact" type="submit" disabled={!canSend || length < 1 || length > 200}>发送</button>
        </div>
        <span className="muted danma-count" id={`${inputID}-count`}>{length}/200 字符</span>
      </form>
    </section>
  );
}
