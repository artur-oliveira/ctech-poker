'use client';

import {useEffect, useRef, useState} from 'react';
import type {TableSnapshot} from '@/lib/api/table';
import {rabbitHuntFee, verifyRabbitCard} from '@/lib/rabbitHunt';

export type RabbitSlotStatus = 'offer' | 'pending' | 'verifying' | 'revealed' | 'failed';
export type RabbitSlot = { status: RabbitSlotStatus; card?: string };

/** What the board needs to draw the rabbit hunt inside its undealt slots. */
export type RabbitHuntBoard = {
  fee: number;
  // Keyed by board slot (0-4); only undealt slots appear.
  slots: Record<number, RabbitSlot>;
  onBuy: (slot: number) => void;
};

type Local = { hand: string; pending: number | null; failCount: number; verified: Record<number, string>; failed: Record<number, true> };

/**
 * Per-card rabbit hunt state for one hand. A card moves offer → pending (the
 * request is in flight) → verifying (the server sent the card and its salt)
 * → revealed, or → failed when the browser cannot rebuild the deck
 * commitment from the proof, in which case that one card's fee is reported
 * back for a refund. Nothing is shown face-up until the proof checks out.
 */
export function useRabbitHunt({snapshot, viewer, failCount = 0, onBuy, onVerifyFailed}: {
  snapshot: TableSnapshot;
  viewer?: string;
  failCount?: number;
  onBuy?: (slot: number) => boolean | void;
  onVerifyFailed?: (slot: number) => void;
}): RabbitHuntBoard | undefined {
  const hand = snapshot.hand_id ?? '';
  const [local, setLocal] = useState<Local>(
    () => ({hand, pending: null, failCount, verified: {}, failed: {}}));
  // A new hand forgets everything; a rejected request (failCount moved)
  // releases the slot so the player can try again. Adjusted during render,
  // not in an effect, so the stale state never paints.
  if (local.hand !== hand) {
    setLocal({hand, pending: null, failCount, verified: {}, failed: {}});
  } else if (local.failCount !== failCount) {
    setLocal({...local, pending: null, failCount});
  }

  const checking = useRef(new Set<string>());
  const cards = snapshot.rabbit_cards;
  useEffect(() => {
    if (!cards) return;
    for (const [key, card] of Object.entries(cards)) {
      const slot = Number(key);
      // Every snapshot decodes a fresh `rabbit_cards` object, so a check is
      // started once per (hand, slot, card) and never cancelled by the next
      // frame; the hand guard below drops a result that outlived its hand.
      const id = `${hand}:${slot}:${card}`;
      if (checking.current.has(id)) continue;
      checking.current.add(id);
      void verifyRabbitCard(snapshot, slot).catch(() => false).then(ok => {
        setLocal(prev => prev.hand !== hand ? prev : ok
          ? {...prev, verified: {...prev.verified, [slot]: card}}
          : {...prev, failed: {...prev.failed, [slot]: true}});
        if (!ok) onVerifyFailed?.(slot);
      });
    }
    // The snapshot is read only for the proof that came with these cards.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cards, hand, onVerifyFailed]);

  const fee = rabbitHuntFee(snapshot, viewer);
  const bought = cards ? Object.keys(cards).length : 0;
  if (!fee && !bought && !Object.keys(local.failed).length) return undefined;

  // A purchase stops being in flight the moment its card arrives.
  const inFlight = local.pending != null && !cards?.[local.pending] ? local.pending : null;
  const slots: Record<number, RabbitSlot> = {};
  for (let slot = snapshot.board.length; slot < 5; slot++) {
    if (local.failed[slot]) slots[slot] = {status: 'failed'};
    else if (local.verified[slot]) slots[slot] = {status: 'revealed', card: local.verified[slot]};
    else if (cards?.[slot]) slots[slot] = {status: 'verifying'};
    else if (inFlight === slot) slots[slot] = {status: 'pending'};
    else if (fee) slots[slot] = {status: 'offer'};
  }
  return {
    fee,
    slots,
    onBuy: slot => {
      if (inFlight != null) return;
      if (onBuy?.(slot) === false) return;
      setLocal(prev => ({...prev, pending: slot}));
    }
  };
}
