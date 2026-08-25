'use client';

import {Check, Eye, Hourglass, X} from 'lucide-react';
import type {TableSnapshot} from '@/lib/api/table';
import {useCountdownMs} from '@/components/store/useCountdown';

// Paying no longer buys the cards outright — it buys a request the winner has
// to accept, because those cards belong to them and not to the deck
// (docs/specs/2026-08-24-pay-to-see-cards-consent.md). This one panel covers
// all three sides of that exchange: the offer, the wait, and the prompt.
export function WinnerCards({snapshot, viewer, bigBlind, pending, onRequestWinnerCardsAction,
                             onAnswerWinnerCardsAction, offerBlocked = false}: {
  snapshot: TableSnapshot;
  viewer?: string;
  bigBlind: number;
  pending?: boolean;
  onRequestWinnerCardsAction?: () => void;
  onAnswerWinnerCardsAction?: (accept: boolean) => void;
  offerBlocked?: boolean;
}) {
  const request = snapshot.pending_winner_cards;
  // The server only sends the request to the two players it concerns, so its
  // mere presence is the authorization to render either half of the exchange.
  const remainingMs = useCountdownMs(request ? request.expires_at_unix_ms : null);
  const seconds = Math.ceil(remainingMs / 1000);

  if (request && request.winner_id === viewer) {
    return <aside className="winner-cards winner-cards-prompt" aria-live="assertive">
      <p><b>{`${request.requester_name || 'Um jogador'} quer pagar ${request.fee.toLocaleString('pt-BR')} fichas para ver sua mão.`}</b>
        <small>{`Você recebe metade. ${seconds}s para responder — sem resposta, a cobrança é devolvida.`}</small></p>
      <div className="winner-cards-answers">
        <button type="button" disabled={pending} onClick={() => onAnswerWinnerCardsAction?.(false)}>
          <X aria-hidden="true"/> Recusar
        </button>
        <button type="button" className="winner-cards-accept" disabled={pending}
                onClick={() => onAnswerWinnerCardsAction?.(true)}>
          <Check aria-hidden="true"/> Mostrar
        </button>
      </div>
    </aside>;
  }

  if (request && request.requester_id === viewer) {
    return <aside className="winner-cards" aria-live="polite">
      <p className="winner-cards-waiting"><Hourglass aria-hidden="true"/>
        <span><b>Aguardando resposta…</b>
          <small>{`${seconds}s. Se recusar ou não responder, suas ${request.fee.toLocaleString('pt-BR')} fichas voltam.`}</small></span>
      </p>
    </aside>;
  }

  if (offerBlocked || snapshot.stage !== 'complete' || !snapshot.won_without_showdown) return null;
  const winnerID = snapshot.winners?.[0];
  const winner = snapshot.seats.find(seat => seat.player_id === winnerID);
  const viewerParticipated = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  if (!winner || !viewerParticipated || winner.player_id === viewer || !winner.dealt_in ||
    (winner.hole_cards && winner.hole_cards.some(card => card !== 'back'))) return null;

  const winnerName = winner.name || 'vencedor';
  return <aside className="winner-cards" aria-live="polite">
    <button type="button" disabled={pending} onClick={onRequestWinnerCardsAction}>
      <Eye aria-hidden="true"/>
      <span><b>{`Pedir a mão de ${winnerName} por ${bigBlind.toLocaleString('pt-BR')} fichas`}</b>
        <small>{`${winnerName} decide se mostra; se recusar, você recebe as fichas de volta`}</small></span>
    </button>
  </aside>;
}
