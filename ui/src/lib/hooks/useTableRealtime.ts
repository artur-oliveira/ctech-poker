'use client'
import {useCallback, useEffect, useRef, useState} from 'react';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import type {ServerMessage, TableSnapshot} from '@/lib/api/table'

type Status = 'disconnected' | 'connecting' | 'connected' | 'error'

export function useTableRealtime(id: string) {
  const socket = useRef<WebSocket | null>(null), [status, setStatus] = useState<Status>('disconnected'), [snapshot, setSnapshot] = useState<TableSnapshot | null>(null), [unlock, setUnlock] = useState<{
    key: string;
    stars: number
  } | null>(null), [chat, setChat] = useState<{ player: string; message: string }[]>([])
  const receive = useCallback((m: ServerMessage) => {
    if (m.type === 'state' && m.snapshot) setSnapshot(m.snapshot);
    if (m.type === 'achievement_unlocked' && m.key) setUnlock({key: m.key, stars: m.stars || 1});
    if (m.type === 'chat' && m.message) setChat(v => [...v.slice(-39), {
      player: m.player_id || '?',
      message: m.message!
    }])
  }, [])
  useEffect(() => {
    if (!id) return;
    let active = true, retry: ReturnType<typeof setTimeout> | undefined;
    const origin = (process.env.NEXT_PUBLIC_API_URL || window.location.origin).replace(/^http/, 'ws');

    function connect() {
      if (!active) return;
      setStatus('connecting');
      const ws = new WebSocket(`${origin}/v1.0/tables/${encodeURIComponent(id)}/ws`);
      socket.current = ws;
      ws.onopen = () => {
        setStatus('connected');
        const token = getAccessToken();
        if (token) ws.send(JSON.stringify({token}))
      };
      ws.onmessage = e => {
        try {
          receive(JSON.parse(e.data) as ServerMessage)
        } catch {
        }
      };
      ws.onerror = () => setStatus('error');
      ws.onclose = () => {
        if (active) {
          setStatus('disconnected');
          retry = setTimeout(connect, 1500)
        }
      }
    }

    connect();
    const unsubscribe = subscribeAccessToken(() => {
      socket.current?.close()
    });
    return () => {
      active = false;
      if (retry) clearTimeout(retry);
      unsubscribe();
      socket.current?.close(1000);
      socket.current = null
    }
  }, [id, receive])
  const emit = useCallback((value: object) => {
    if (socket.current?.readyState === WebSocket.OPEN) socket.current.send(JSON.stringify(value))
  }, []);
  return {
    status,
    snapshot,
    unlock,
    chat,
    ready: (ready = true) => emit({type: 'ready', ready}),
    act: (action: string, amount = 0) => emit({type: 'act', action, amount, action_id: crypto.randomUUID()}),
    sendChat: (message: string) => emit({type: 'chat', message})
  }
}
