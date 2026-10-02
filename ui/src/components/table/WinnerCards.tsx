'use client';

import {Check, Eye, Hourglass, X} from 'lucide-react';
import type {WinnerCardsRequest} from '@/lib/api/table';
import {useCountdownMs} from '@/components/store/useCountdown';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {useChipExact, useChipFormat, useChipUnit} from '@/lib/chipFormat';
import type {WinnerCardsSeat} from '@/lib/winnerCards';

// Paying never buys the cards outright — it buys a request the winner has to
// accept, because those cards belong to them and not to the deck
// (docs/specs/2026-08-24-pay-to-see-cards-consent.md). Both halves of the
// exchange live on the table itself (docs/specs/2026-10-01-paid-reveals-on-table.md):
// the requester clicks the winner's face-down cards, and the winner answers
// at their own seat.

const PROMPT_AVATARS = 3;

function useSecondsLeft(expiresAtMs: number | null) {
  return Math.ceil(useCountdownMs(expiresAtMs) / 1000);
}

/** Laid over the uncontested winner's face-down cards for a viewer who may
 * ask to see them: the cards become the button, the price sits under them,
 * and once asked the same spot holds the wait. */
export function WinnerCardsRequestControl({state, winnerName, fee, pending, onRequest}: {
  state: Extract<WinnerCardsSeat, { mode: 'offer' | 'waiting' }>;
  winnerName: string;
  fee: number;
  pending?: boolean;
  onRequest?: () => void;
}) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  if (state.mode === 'waiting') return <WinnerCardsWait request={state.request} winnerName={winnerName}/>;
  if (pending) {
    return <span className="winner-cards-request" data-state="waiting" role="status"
                 aria-label={`Enviando o pedido para ver as cartas de ${winnerName}…`}>
      <Hourglass aria-hidden="true"/>
    </span>;
  }
  return <button type="button" className="winner-cards-request" data-state="offer" onClick={onRequest}
                 aria-label={`Pedir para ver as cartas de ${winnerName} por ${exact(fee)}${unit}. ${winnerName} decide se mostra; se recusar, a cobrança é devolvida.`}>
    <span className="winner-cards-price" aria-hidden="true"><Eye/>{chips(fee)}</span>
  </button>;
}

// Its own component so the countdown's clock starts when the wait does, not
// when the offer first mounted.
function WinnerCardsWait({request, winnerName}: { request: WinnerCardsRequest; winnerName: string }) {
  const exact = useChipExact();
  const unit = useChipUnit();
  const seconds = useSecondsLeft(request.expires_at_unix_ms);
  return <span className="winner-cards-request" data-state="waiting" role="status"
               aria-label={`Aguardando ${winnerName} responder, ${seconds}s. Se recusar ou não responder, a cobrança de ${exact(request.fee)}${unit} volta para você.`}>
    <Hourglass aria-hidden="true"/>
    <b aria-hidden="true">{seconds}s</b>
  </span>;
}

function askersLine(requests: WinnerCardsRequest[]) {
  const names = requests.map(request => request.requester_name || 'Um jogador');
  if (names.length === 1) return `${names[0]} quer ver sua mão`;
  if (names.length === 2) return `${names[0]} e ${names[1]} querem ver sua mão`;
  return `${names[0]}, ${names[1]} e mais ${names.length - 2} querem ver sua mão`;
}

/** The winner's one answer for the whole batch, anchored at their own seat. */
export function WinnerCardsPrompt({requests, pending, onAnswer}: {
  requests: WinnerCardsRequest[];
  pending?: boolean;
  onAnswer?: (accept: boolean) => void;
}) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  const seconds = useSecondsLeft(requests[0]?.expires_at_unix_ms ?? null);
  // The winner keeps half of every fee on accept (the rest is rake).
  const total = requests.reduce((sum, request) => sum + Math.floor(request.fee / 2), 0);
  const shown = requests.slice(0, PROMPT_AVATARS);
  const extra = requests.length - shown.length;
  return <div className="winner-cards-prompt" role="group" aria-label="Pedido para ver sua mão">
    <div className="winner-cards-head">
      <span className="winner-cards-askers" aria-hidden="true">
        {shown.map(request => <PlayerAvatar key={request.requester_id} name={request.requester_name}
                                            avatarUrl={request.requester_avatar_url} decorative/>)}
        {extra > 0 && <span className="winner-cards-more">+{extra}</span>}
      </span>
      <p>
        <b role="alert">{askersLine(requests)}</b>
        <small>
          <span aria-label={`Você recebe ${exact(total)}${unit}`}>Você recebe {chips(total)}</span>
          {' · '}
          <span className="winner-cards-seconds" aria-label={`${seconds} segundos para responder`}>{seconds}s</span>
        </small>
      </p>
    </div>
    <div className="winner-cards-answers">
      <button type="button" disabled={pending} onClick={() => onAnswer?.(false)}
              aria-label="Recusar: ninguém vê sua mão e a cobrança é devolvida">
        <X aria-hidden="true"/> Recusar
      </button>
      <button type="button" className="winner-cards-accept" disabled={pending} onClick={() => onAnswer?.(true)}
              aria-label={`Mostrar sua mão a quem pediu e receber ${exact(total)}${unit}`}>
        <Check aria-hidden="true"/> Mostrar
      </button>
    </div>
  </div>;
}
