'use client';

import {useCallback, useEffect, useRef, useState} from 'react';
import {MAX_RECONNECT_ATTEMPTS, useWebSocket, type WSStatus} from '@aoctech/ws-client';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {checkApiLiveness} from '@/lib/network/liveness';
import {useApiLiveness} from '@/lib/network/NetworkProvider';
import {type MockScenario, USE_MOCK} from '@/lib/mockConfig';
import {wsOrigin} from '@/lib/ws/origin';
import {decodeServerMessage, encodeClientMessage} from '@/lib/ws/utils';
import type {ServerMessage} from '@/lib/api/table';
import type {MockTableService} from '@/dev/mockRuntime';

export type TableSocketMockOptions = {scenario?: MockScenario; delay?: number};

type TableSocketOptions = {
  id: string;
  shareCode?: string;
  terminalError: string | null;
  mockOptions?: TableSocketMockOptions;
  onMessage: (message: ServerMessage) => void;
  onOpen: () => void;
  onMockReset: () => void;
  onMockConnecting: () => void;
};

/** Owns transport only: auth-token subscription, real/mock socket lifecycle,
 * liveness gating, reconnect status and background-tab recovery. Table state
 * and command policy deliberately stay outside this boundary. */
export function useTableSocket(options: TableSocketOptions) {
  const {id, shareCode, terminalError, mockOptions, onMessage, onOpen,
    onMockReset, onMockConnecting} = options;
  const [socketAuthToken, setSocketAuthToken] = useState(() => getAccessToken());
  const apiLiveness = useApiLiveness();
  const [mockStatus, setMockStatus] = useState<WSStatus>('connecting');
  const [mockReconnectAttempt, setMockReconnectAttempt] = useState(0);
  const mockService = useRef<MockTableService | null>(null);

  useEffect(() => subscribeAccessToken(setSocketAuthToken), []);

  const wsUrl = id ? `${wsOrigin()}/v1.0/tables/${encodeURIComponent(id)}/ws` : null;
  const {status: wsStatus, attempt: wsReconnectAttempt, send: wsSend, reconnect: wsRetryNow} = useWebSocket({
    url: wsUrl,
    binaryType: 'arraybuffer',
    encode: encodeClientMessage,
    decode: decodeServerMessage,
    onMessage: data => onMessage(data as ServerMessage),
    enabled: Boolean(wsUrl) && !USE_MOCK && Boolean(socketAuthToken) &&
      apiLiveness.status === 'available' && !terminalError,
    authToken: socketAuthToken || undefined,
    shareCode,
    onOpen
  });

  useEffect(() => {
    if (!USE_MOCK && socketAuthToken && (wsStatus === 'error' || wsStatus === 'disconnected')) {
      void checkApiLiveness();
    }
  }, [socketAuthToken, wsStatus]);

  const mockScenario = mockOptions?.scenario || 'flop';
  const mockDelay = Math.min(15000, Math.max(0, mockOptions?.delay ?? 650));
  useEffect(() => {
    if (!USE_MOCK || !id) return () => {};
    onMockReset();
    let service: MockTableService | null = null;
    let cancelled = false;
    void import('@/dev/mockRuntime').then(({MockTableService}) => {
      if (cancelled) return;
      service = new MockTableService(mockScenario, mockDelay, {
        onMessage,
        onStatus: (next, attempt) => {
          if (next === 'connecting') onMockConnecting();
          setMockStatus(next);
          setMockReconnectAttempt(attempt);
        }
      });
      mockService.current = service;
      service.connect();
    });
    return () => {
      cancelled = true;
      service?.close();
      if (mockService.current === service) mockService.current = null;
    };
  }, [id, mockDelay, mockScenario, onMessage, onMockConnecting, onMockReset]);

  const send = useCallback((value: object) => USE_MOCK
    ? Boolean(mockService.current?.send(value as Record<string, unknown>))
    : wsSend(value), [wsSend]);
  const retryNow = useCallback(() => USE_MOCK
    ? mockService.current?.reconnect()
    : wsRetryNow(), [wsRetryNow]);
  const reconnectAttempt = USE_MOCK ? mockReconnectAttempt : wsReconnectAttempt;
  const rawStatus = USE_MOCK ? mockStatus
    : apiLiveness.status === 'unavailable' ? 'disconnected' : wsStatus;
  const status: WSStatus = (rawStatus === 'error' || rawStatus === 'disconnected') &&
    reconnectAttempt <= MAX_RECONNECT_ATTEMPTS ? 'reconnecting' : rawStatus;

  useEffect(() => {
    if (USE_MOCK || typeof document === 'undefined') return () => {};
    const onVisibility = () => {
      if (document.visibilityState === 'visible' && status !== 'connected') retryNow();
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [status, retryNow]);

  return {status, reconnectAttempt, send, retryNow};
}
