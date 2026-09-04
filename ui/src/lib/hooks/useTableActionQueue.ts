'use client';

import {useCallback, useEffect, useRef} from 'react';
import {auxRetryDelayMs, MAX_ACTION_RETRIES, RESYNC_ERROR_CODES} from '@/lib/tableResilience';
import type {PokerAction} from '@/lib/api/table';

export type PendingTableAction = {
  id: string;
  action: PokerAction;
  amount: number;
  snapshotVersion: number;
  handId: string;
  retries: number;
  awaitingRetry: boolean;
};

type QueuedAuxiliaryAction = {
  frame: object;
  retries: number;
  timer?: ReturnType<typeof setTimeout>;
};

export type AuxiliaryRetryPlan = {frame: object; retries: number; delayMs: number};

export function shouldRetryPendingAction(action: PendingTableAction | null, code: string,
  actionId?: string): action is PendingTableAction {
  return code === 'stale_state' && Boolean(actionId) && action !== null && action.id === actionId &&
    action.retries < MAX_ACTION_RETRIES;
}

export function advancePendingAction(action: PendingTableAction, snapshotVersion: number,
  handId: string): PendingTableAction {
  return {...action, snapshotVersion, handId, retries: action.retries + 1, awaitingRetry: false};
}

export function resyncDelayMs(code: string, retriesUsed: number, jitter = Math.random()) {
  const backoffMs = code === 'rate_limited' ? 800 : Math.min(1600, 50 * 2 ** retriesUsed);
  return backoffMs + Math.floor(jitter * 400);
}

/** Pure retry decision used by the queue and its unit tests. */
export function planAuxiliaryRetry(action: Readonly<QueuedAuxiliaryAction> | undefined,
  code: string, jitter = Math.random()): AuxiliaryRetryPlan | null {
  if (!RESYNC_ERROR_CODES.has(code) || !action || action.retries >= MAX_ACTION_RETRIES) return null;
  const retries = action.retries + 1;
  return {frame: action.frame, retries, delayMs: auxRetryDelayMs(retries, jitter)};
}

/** Owns auxiliary frame correlation and retry timers. The session tells it
 * what to send and how to surface terminal delivery failure; it never knows
 * about snapshots, React view state, or the socket implementation. */
export function useTableActionQueue(send: (frame: object) => boolean,
  onDeliveryFailure: (actionId: string) => void) {
  const actions = useRef(new Map<string, QueuedAuxiliaryAction>());
  const sendRef = useRef(send);
  const failureRef = useRef(onDeliveryFailure);
  useEffect(() => { sendRef.current = send; }, [send]);
  useEffect(() => { failureRef.current = onDeliveryFailure; }, [onDeliveryFailure]);

  const drop = useCallback((actionId: string) => {
    const action = actions.current.get(actionId);
    if (action?.timer) clearTimeout(action.timer);
    actions.current.delete(actionId);
  }, []);

  const track = useCallback((actionId: string, frame: object) => {
    drop(actionId);
    actions.current.set(actionId, {frame, retries: 0});
  }, [drop]);

  const retry = useCallback((actionId: string, code: string) => {
    const action = actions.current.get(actionId);
    const plan = planAuxiliaryRetry(action, code);
    if (!plan || !action) return false;
    action.retries = plan.retries;
    if (action.timer) clearTimeout(action.timer);
    action.timer = setTimeout(() => {
      action.timer = undefined;
      if (!actions.current.has(actionId)) return;
      if (!sendRef.current(plan.frame)) failureRef.current(actionId);
    }, plan.delayMs);
    return true;
  }, []);

  const clear = useCallback(() => {
    for (const action of actions.current.values()) if (action.timer) clearTimeout(action.timer);
    actions.current.clear();
  }, []);
  useEffect(() => clear, [clear]);

  return {track, drop, retry, clear};
}
