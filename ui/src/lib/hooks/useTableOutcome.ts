'use client';
import {useEffect, useRef, useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import type {TableSnapshot} from '@/lib/api/table';
import type {HandOutcomeState} from '@/components/table/HandOutcome';
import {buildHandOutcome, seatParticipated} from '@/lib/tableOutcome';
import {invalidateAfterSettle} from '@/lib/settleRefetch';

/** The showdown banner and the two frozen timestamps around it.
 *
 * Everything the banner actually says is assembled by `buildHandOutcome`; this
 * hook owns only the bookkeeping that cannot be derived from the current frame:
 * which payout is new, which starting stack the hand began with, and when the
 * next-hand deadline was armed. */
export function useTableOutcome({id, viewer, snapshot, snapshotAt}: {
  id: string;
  viewer?: string;
  snapshot: TableSnapshot | null;
  snapshotAt: number;
}) {
  const queryClient = useQueryClient();
  // Protocol v3 publishes the exact pre-blind stack. During a rolling deploy,
  // remember the earliest live snapshot as stack+contributed; unlike the old
  // stage-based state this is scoped to both table and hand and also works
  // when the first frame arrives on flop/turn after a reconnect.
  const [rememberedStart, setRememberedStart] =
    useState<{ tableID: string; handID: string; stack: number } | null>(null);
  // The next-hand deadline is fixed server-side once armed, but a state
  // broadcast can still arrive mid-countdown (e.g. another player revealing
  // cards) and shift snapshotAt forward. Recomputing the animation duration
  // against that later snapshotAt would shrink the CSS animation's total
  // duration while it's already running, snapping the ring to its end frame
  // long before the real deadline. Freezing the duration at the first snapshot
  // that armed this deadline keeps the ring in sync with backend time
  // regardless of how many broadcasts land before it fires.
  const [nextHandArmed, setNextHandArmed] = useState<{ deadline: number; snapshotAt: number } | null>(null);
  // Fires the win/lose banner exactly once per resolved hand: payouts appear
  // once when a hand completes and stay put across every later broadcast of
  // that same `complete` snapshot (show_cards, pings, ...), so comparing
  // against the previous render's payouts (not the current one) is what keeps
  // this from re-firing on those repeats.
  const previousPayoutsRef = useRef<{ tableID: string; payouts?: TableSnapshot['payouts'] }>({tableID: ''});
  const outcomeKeyRef = useRef(0);
  const [scopedHandOutcome, setScopedHandOutcome] =
    useState<{ tableID: string; handID?: string; value: HandOutcomeState } | null>(null);

  const liveSeat = snapshot?.seats.find(item => item.player_id === viewer);
  useEffect(() => {
    const handID = snapshot?.hand_id;
    if (!handID || !liveSeat || !seatParticipated(liveSeat) || Object.keys(snapshot?.payouts || {}).length) return;
    const next = {
      tableID: id, handID,
      stack: liveSeat.stack_at_hand_start ?? liveSeat.stack + liveSeat.contributed,
    };
    // This state intentionally retains the starting stack after payouts remove
    // it from the live snapshot; it cannot be derived from the current frame.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setRememberedStart(previous => previous?.tableID === id && previous.handID === handID ? previous : next);
  }, [id, liveSeat, snapshot?.hand_id, snapshot?.payouts]);

  useEffect(() => {
    const deadline = snapshot?.next_hand_unix_ms;
    if (!deadline) return;
    // Preserve the timestamp of the first frame carrying this deadline.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setNextHandArmed(previous => previous?.deadline === deadline ? previous : {deadline, snapshotAt});
  }, [snapshot?.next_hand_unix_ms, snapshotAt]);

  useEffect(() => {
    const hasPayouts = Boolean(snapshot?.payouts && Object.keys(snapshot.payouts).length > 0);
    const previousPayouts = previousPayoutsRef.current.tableID === id ?
      previousPayoutsRef.current.payouts : undefined;
    const isFreshPayout = hasPayouts && !previousPayouts;
    previousPayoutsRef.current = {tableID: id, payouts: hasPayouts ? snapshot?.payouts : undefined};
    if (!isFreshPayout || !snapshot || !viewer) return;
    const remembered = rememberedStart?.tableID === id ? rememberedStart : null;
    const value = buildHandOutcome(snapshot, viewer, remembered, outcomeKeyRef.current + 1);
    if (!value) return;
    outcomeKeyRef.current += 1;
    setScopedHandOutcome({tableID: id, handID: snapshot.hand_id, value});
  }, [snapshot, viewer, id, rememberedStart]);

  // The banner/toast-blocking state above is scoped to the hand that produced
  // it and must not survive into the next one: once the server deals a new
  // hand (a fresh hand_id), whatever outcome is still displayed belongs to the
  // previous hand's exit animation only (handled locally by HandOutcomeBanner)
  // and no longer has any business gating achievement toasts or the
  // pay-to-see-winner-cards offer. Without this it latches non-null forever
  // after the very first resolved hand.
  useEffect(() => {
    const handID = snapshot?.hand_id;
    if (!handID) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setScopedHandOutcome(previous => previous && previous.tableID === id && previous.handID !== handID ?
      null : previous);
  }, [snapshot?.hand_id, id]);

  // The last-winners strip reads the player's own hand history, which the
  // server only writes on a pipeline it runs *after* broadcasting this
  // `complete` snapshot — so one invalidate here would refetch before the row
  // exists. Re-invalidate with a backoff, keyed on the settled hand so the
  // sequence starts once and is cancelled when the next hand deals.
  const settledHandID = snapshot?.payouts && Object.keys(snapshot.payouts).length > 0 ?
    snapshot?.hand_id : undefined;
  useEffect(() => {
    if (!settledHandID) return undefined;
    return invalidateAfterSettle(queryClient, ['hands', id]);
  }, [settledHandID, id, queryClient]);

  const viewerSeat = snapshot?.seats.find(seat => seat.player_id === viewer);
  return {
    handOutcome: scopedHandOutcome?.tableID === id ? scopedHandOutcome.value : null,
    viewerStackBefore: viewerSeat?.stack_at_hand_start ??
      (rememberedStart?.tableID === id && rememberedStart.handID === snapshot?.hand_id ?
        rememberedStart.stack : undefined),
    nextHandDurationMs: snapshot?.next_hand_unix_ms && nextHandArmed?.deadline === snapshot.next_hand_unix_ms ?
      Math.max(0, snapshot.next_hand_unix_ms - nextHandArmed.snapshotAt) : 0
  };
}
