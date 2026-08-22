'use client';

import {Eye} from 'lucide-react';
import type {TableSnapshot} from '@/lib/api/table';

export function WinnerCards({snapshot, viewer, bigBlind, pending, onRequestWinnerCardsAction}: {
  snapshot: TableSnapshot;
  viewer?: string;
  bigBlind: number;
  pending?: boolean;
  onRequestWinnerCardsAction?: () => void;
}) {
  if (snapshot.stage !== 'complete' || !snapshot.won_without_showdown) return null;
  const winnerID = snapshot.winners?.[0];
  const winner = snapshot.seats.find(seat => seat.player_id === winnerID);
  const viewerParticipated = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  if (!winner || !viewerParticipated || winner.player_id === viewer || !winner.dealt_in ||
    (winner.hole_cards && winner.hole_cards.some(card => card !== 'back'))) return null;

  const winnerName = winner.name || 'vencedor';
  return <aside className="winner-cards" aria-live="polite">
    <button type="button" disabled={pending} onClick={onRequestWinnerCardsAction}>
      <Eye aria-hidden="true"/>
      <span><b>{`Ver a mão de ${winnerName} por ${bigBlind.toLocaleString('pt-BR')} fichas`}</b>
        <small>As cartas aparecem apenas para você</small></span>
    </button>
  </aside>;
}
