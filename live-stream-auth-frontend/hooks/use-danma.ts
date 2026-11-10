'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { roomsApi } from '../lib/api';
import { isRoomPageSuspended, leaveCurrentRoom, ownRoomConnection, resumeRoomPage } from '../lib/room-lifecycle';
import { mergeDanma } from '../lib/message-history';
import type { DanmaMessage } from '../lib/message-history';
import { createDanma, parseServerMessage, roomWebSocketURL } from '../lib/websocket';
import { DanmaOutbox } from '../lib/danma-outbox';
import type { OutgoingDanma } from '../lib/danma-outbox';
import type { ClientDanma } from '../lib/websocket';

export type ConnectionStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

// userID 仅控制 UI；服务端通过 sid 确认身份。未开播房间只读历史。
export function useDanma(roomID: number, userID: number | null, live = true, initialLikeCount = 0) {
  const outbox = useMemo(() => new DanmaOutbox(roomID, userID), [roomID, userID]);
  const [outgoing, setOutgoing] = useState<OutgoingDanma[]>([]);
  const socketRef = useRef<WebSocket | null>(null);
  const loadMoreRef = useRef<() => Promise<boolean>>(async () => false);
  const refreshLikesRef = useRef<() => Promise<void>>(async () => {});
  const nextLikeAtRef = useRef(0);
  const likeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [likeCount, setLikeCount] = useState(initialLikeCount);
  const [onlineCount, setOnlineCount] = useState<number | null>(null);
  const [likeCooling, setLikeCooling] = useState(false);
  const [likeSyncing, setLikeSyncing] = useState(false);
  const [likeError, setLikeError] = useState('');
  const [status, setStatus] = useState<ConnectionStatus>(live ? 'connecting' : 'idle');
  const [messages, setMessages] = useState<DanmaMessage[]>([]);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [historyError, setHistoryError] = useState('');
  const [historyLoading, setHistoryLoading] = useState(true);
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [historyVersion, setHistoryVersion] = useState(0);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const unsubscribe = outbox.subscribe(() => setOutgoing(outbox.snapshot()));
    setOutgoing(outbox.snapshot());
    return () => { unsubscribe(); outbox.pause(); };
  }, [outbox]);

  useEffect(() => {
    setLikeCount(current => Math.max(current, initialLikeCount));
  }, [initialLikeCount]);

  useEffect(() => {
    let disposed = false;
    let socket: WebSocket | null = null;
    let busy = false;
    let started = false;
    let loaded = false;
    let more = false;
    let cursor = 0;
    let syncingLikes = false;
    const request = new AbortController();
    setStatus(live ? 'connecting' : 'idle');
    setMessages([]);
    setNotice('');
    setError('');
    setHistoryError('');
    setHistoryLoading(true);
    setHistoryLoaded(false);
    setHasMore(false);
    setHistoryVersion(0);
    setLikeCooling(false);
    setOnlineCount(null);
    setLikeSyncing(false);
    setLikeError('');
    nextLikeAtRef.current = 0;

    async function refreshLikes() {
      if (disposed || syncingLikes) return;
      syncingLikes = true;
      setLikeSyncing(true);
      setLikeError('');
      try {
        const room = await roomsApi.detail(roomID, request.signal);
        if (!disposed) setLikeCount(current => Math.max(current, room.like_count));
      } catch (error) {
        if (!disposed) setLikeError(error instanceof Error ? error.message : '刷新点赞数失败。');
      } finally {
        syncingLikes = false;
        if (!disposed) setLikeSyncing(false);
      }
    }
    refreshLikesRef.current = refreshLikes;

    async function loadPage(beforeID?: number): Promise<boolean> {
      if (disposed || busy) return false;
      busy = true;
      setHistoryLoading(true);
      setHistoryError('');
      try {
        const page = await roomsApi.messages(roomID, beforeID, request.signal);
        if (disposed) return false;
        let confirmed = false;
        for (const message of page.data) {
          if (outbox.confirm(message)) confirmed = true;
        }
        if (confirmed) {
          setError('');
          setNotice('弹幕已保存');
        }
        setMessages(current => mergeDanma(current, page.data));
        loaded = true;
        more = page.has_more;
        cursor = page.next_before_id;
        setHistoryLoaded(true);
        setHasMore(more);
        setHistoryVersion(current => current + 1);
        return true;
      } catch (error) {
        if (!disposed) setHistoryError(error instanceof Error ? error.message : '加载历史消息失败。');
        return false;
      } finally {
        busy = false;
        if (!disposed) setHistoryLoading(false);
      }
    }

    function startHistory() {
      if (disposed || started) return;
      started = true;
      void loadPage();
      void refreshLikes();
    }

    const loadMore = () => {
      if (loaded && !more) return Promise.resolve(false);
      return loadPage(loaded ? cursor : undefined);
    };
    loadMoreRef.current = loadMore;

    function stopConnection(): boolean {
      if (disposed) return false;
      disposed = true;
      outbox.pause();
      request.abort();
      if (likeTimerRef.current !== null) clearTimeout(likeTimerRef.current);
      likeTimerRef.current = null;
      nextLikeAtRef.current = 0;
      if (refreshLikesRef.current === refreshLikes) refreshLikesRef.current = async () => {};
      if (loadMoreRef.current === loadMore) loadMoreRef.current = async () => false;
      if (socketRef.current === socket) socketRef.current = null;
      if (socket) {
        socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null;
        if (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN) {
          socket.close(1000, '离开房间或重新连接');
        }
      }
      return true;
    }

    const release = ownRoomConnection(() => {
      if (stopConnection()) {
        setStatus('closed');
        setOnlineCount(null);
      }
    });
    const onPageHide = () => leaveCurrentRoom();
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted) {
        resumeRoomPage();
        setRevision(current => current + 1);
      }
    };
    window.addEventListener('pagehide', onPageHide);
    window.addEventListener('pageshow', onPageShow);

    if (isRoomPageSuspended()) {
      setStatus('closed');
      setHistoryLoading(false);
    } else if (live) {
      try {
        socket = new WebSocket(roomWebSocketURL(roomID));
        socketRef.current = socket;
        socket.onopen = () => {
          if (disposed) return;
          setStatus('open');
          // 先建立实时连接，再查最近历史，交界消息通过 UUID 合并。
          startHistory();
        };
        socket.onmessage = event => {
          if (disposed) return;
          try {
            const message = parseServerMessage(event.data);
            if (message.room_id !== roomID) throw new Error('收到房间不匹配的消息。');
            if (message.type === 'error') {
              const handled = outbox.fail(message);
              // 已确认/已移除消息的迟到关联错误不覆盖当前提示。
              if (!message.message_id || handled) setError(message.content);
            }
            else if (message.type === 'system') setNotice(message.content);
            else if (message.type === 'accepted') {
              if (message.user_id !== userID) throw new Error('收到身份不匹配的接收回执。');
              if (outbox.accept(message)) {
                setError('');
                setNotice('弹幕已接收，等待保存');
              }
            }
            else if (message.type === 'ack') {
              if (message.user_id !== userID) throw new Error('收到身份不匹配的保存回执。');
              if (outbox.confirm(message)) setError('');
              // ACK 携带数据库原记录；首次发送和重复确认按同一 UUID 合并。
              setMessages(current => mergeDanma(current, [{ ...message, type: 'danma' } as DanmaMessage]));
              setNotice('弹幕已保存');
            }
            else if (message.type === 'room_state') {
              if (message.like_count !== undefined) setLikeCount(current => Math.max(current, message.like_count!));
              if (message.online_count !== undefined) setOnlineCount(message.online_count);
            } else {
              if (outbox.confirm(message)) {
                setError('');
                setNotice('弹幕已保存');
              }
              setMessages(current => mergeDanma(current, [message as DanmaMessage]));
            }
          } catch (error) {
            setError(error instanceof Error ? error.message : '读取消息失败。');
          }
        };
        socket.onerror = () => {
          if (disposed) return;
          setStatus('error');
          outbox.pause();
          setOnlineCount(null);
          setError('实时连接异常，请检查网络、登录状态和房间状态后重新连接。');
          startHistory();
        };
        socket.onclose = event => {
          if (disposed) return;
          setStatus('closed');
          outbox.pause();
          setOnlineCount(null);
          setError(current => current || `实时连接已断开（${event.code}），可以手动重新连接。`);
          startHistory();
        };
      } catch (error) {
        setStatus('error');
        setError(error instanceof Error ? error.message : '创建连接失败。');
        startHistory();
      }
    } else {
      startHistory();
    }

    return () => {
      window.removeEventListener('pagehide', onPageHide);
      window.removeEventListener('pageshow', onPageShow);
      stopConnection();
      release();
    };
  }, [roomID, userID, live, revision, outbox]);

  function send(content: string): boolean {
    if (userID === null || !live) {
      setError(userID === null ? '请先登录后发送弹幕。' : '房间尚未开播。');
      return false;
    }
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      setError('连接尚未就绪，请重新连接后再发送。');
      return false;
    }
    let message: ClientDanma;
    try {
      // 先登记不可变的原消息，再发送；重试沿用这个对象的 UUID 和内容。
      message = outbox.add(createDanma(content));
    } catch (error) {
      setError(error instanceof Error ? error.message : '准备弹幕失败。');
      return false;
    }
    setNotice('');
    try {
      socket.send(JSON.stringify(message));
      setError('');
    } catch (error) {
      outbox.unknown(message.message_id, '发送中断，保存结果尚未确认；请重试原消息。');
      setError(error instanceof Error ? error.message : '发送中断，请在待处理列表重试。');
    }
    // 已经保留在待处理列表中，清空输入框以免误发成一个新 UUID。
    return true;
  }

  function retry(messageID: string) {
    const socket = socketRef.current;
    if (userID === null || !live || !socket || socket.readyState !== WebSocket.OPEN) {
      setError('请先恢复登录和房间连接，再重试原消息。');
      return;
    }
    const message = outbox.retry(messageID);
    if (!message) return;
    setNotice('');
    try {
      socket.send(JSON.stringify(message));
      setError('');
    } catch (error) {
      outbox.unknown(messageID, '重试中断，仍保留原 UUID；请恢复连接后再试。');
      setError(error instanceof Error ? error.message : '重试中断。');
    }
  }

  function like() {
    if (userID === null || !live) {
      setLikeError(userID === null ? '请先登录后点赞。' : '房间尚未开播。');
      return;
    }
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      setLikeError('连接尚未就绪，请重新连接后点赞。');
      return;
    }
    const now = performance.now();
    if (now < nextLikeAtRef.current) return;
    try {
      socket.send(JSON.stringify({ type: 'like' }));
      nextLikeAtRef.current = now + 250;
      setLikeCooling(true);
      setLikeError('');
      setError('');
      likeTimerRef.current = setTimeout(() => {
        likeTimerRef.current = null;
        setLikeCooling(false);
      }, 250);
    } catch (error) {
      setLikeError(error instanceof Error ? error.message : '提交点赞失败。');
    }
  }

  return {
    likeCount, onlineCount, likeCooling, likeSyncing, likeError, like, refreshLikes: () => refreshLikesRef.current(),
    status, messages, outgoing, retry, removeOutgoing: (id: string) => outbox.remove(id),
    notice, error, historyError, historyLoading, historyLoaded, hasMore, historyVersion,
    send, reconnect: () => { resumeRoomPage(); setRevision(current => current + 1); }, loadMore: () => loadMoreRef.current(),
  };
}
