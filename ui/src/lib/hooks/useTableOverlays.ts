'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {isPlainKey} from '@/lib/utils';
import type {TableUtility} from '@/components/table/TableUtilityMenu';
import type {TableReactionID} from '@/lib/reactions';

const REACTION_COOLDOWN_MS = 2000;

/** The table's local chatter: which aside owns the one shared slot, the two
 * standalone dialogs, and the reaction send cooldown. None of it is server
 * state, and none of it belongs in the page body. */
export function useTableOverlays({connected, sendReaction}: {
  connected: boolean;
  sendReaction: (reaction: TableReactionID, playerId?: string) => boolean;
}) {
  const [activeTablePanel, setActiveTablePanel] = useState<TableUtility | null>(null);
  const [preferencesOpen, setPreferencesOpen] = useState(false);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [pendingReaction, setPendingReaction] = useState<TableReactionID | null>(null);
  const [reactionCoolingDown, setReactionCoolingDown] = useState(false);
  const reactionCooldownRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (reactionCooldownRef.current) clearTimeout(reactionCooldownRef.current);
  }, []);

  // The asides share one slot, and a hover-opened one closes on a delay
  // (HOVER_PANEL_CLOSE_DELAY_MS). Crossing the reactions toggle on the way to
  // chat therefore fires reactions' close *after* chat already took the slot,
  // which shut chat again. Only the panel that still owns the slot may clear it.
  // Cached per panel, not rebuilt per call: `panelOpenChange('chat')` is read
  // during render and handed to a memoised aside, so a fresh closure each time
  // would re-render Chat/TableReactions/LastWinners on every snapshot (#230).
  const panelHandlers = useRef(new Map<TableUtility, (open: boolean) => void>());
  const panelOpenChange = useCallback((panel: TableUtility) => {
    const cached = panelHandlers.current.get(panel);
    if (cached) return cached;
    const handler = (open: boolean) =>
      setActiveTablePanel(current => open ? panel : current === panel ? null : current);
    panelHandlers.current.set(panel, handler);
    return handler;
  }, []);

  // E/T open the two asides the player reaches for mid-hand, matching the
  // action bar's own single-letter shortcuts (f/c/p/a/h/r, and 1/2 to peek).
  // isPlainKey keeps them out of the chat input the T panel focuses on open.
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (!isPlainKey(event)) return;
      const panel: TableUtility | null =
        event.key.toLowerCase() === 'e' ? 'reactions' : event.key.toLowerCase() === 't' ? 'chat' : null;
      if (!panel) return;
      event.preventDefault();
      setActiveTablePanel(current => current === panel ? null : panel);
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const startReactionCooldown = useCallback(() => {
    setReactionCoolingDown(true);
    if (reactionCooldownRef.current) clearTimeout(reactionCooldownRef.current);
    reactionCooldownRef.current = setTimeout(() => setReactionCoolingDown(false), REACTION_COOLDOWN_MS);
  }, []);

  // The three commands are `useCallback`s rather than object methods so the
  // memoised surfaces that receive them (TableUtilityMenu, TableReactions,
  // TableStage) only re-render when something they show actually changed.
  const selectTableUtility = useCallback((utility: TableUtility) => {
    if (utility === 'preferences') {
      setPreferencesOpen(true);
      return;
    }
    if (utility === 'share') {
      setInviteOpen(true);
      return;
    }
    setActiveTablePanel(previous => previous === utility ? null : utility);
  }, []);

  const sendQuickReaction = useCallback((reaction: TableReactionID) => {
    if (reactionCoolingDown || !connected) return;
    if (sendReaction(reaction)) startReactionCooldown();
  }, [connected, reactionCoolingDown, sendReaction, startReactionCooldown]);

  const sendTargetedReaction = useCallback((playerId: string) => {
    if (!pendingReaction || reactionCoolingDown || !connected) return;
    if (sendReaction(pendingReaction, playerId)) {
      setPendingReaction(null);
      startReactionCooldown();
    }
  }, [connected, pendingReaction, reactionCoolingDown, sendReaction, startReactionCooldown]);

  return {
    activeTablePanel,
    setActiveTablePanel,
    panelOpenChange,
    preferencesOpen,
    setPreferencesOpen,
    inviteOpen,
    setInviteOpen,
    pendingReaction,
    setPendingReaction,
    reactionCoolingDown,
    selectTableUtility,
    sendQuickReaction,
    sendTargetedReaction
  };
}
