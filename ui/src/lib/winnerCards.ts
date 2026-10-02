import type {TableSnapshot, WinnerCardsRequest} from '@/lib/api/table';
import {PER_CARD_REVEALS_PROTOCOL} from '@/lib/rabbitHunt';

/** What the uncontested winner's seat shows this viewer about the paid
 * winner-cards consent (docs/specs/2026-10-01-paid-reveals-on-table.md):
 * - `offer`: the winner's face-down cards are a button that sends the request;
 * - `waiting`: this viewer asked and the winner has not answered yet;
 * - `prompt`: this viewer IS the winner, and someone is asking. */
export type WinnerCardsSeat =
  | { winnerId: string; mode: 'offer' }
  | { winnerId: string; mode: 'waiting'; request: WinnerCardsRequest }
  | { winnerId: string; mode: 'prompt'; requests: WinnerCardsRequest[] };

export function winnerCardsSeat(snapshot: TableSnapshot, viewer?: string): WinnerCardsSeat | null {
  if (!viewer || (snapshot.protocol_version ?? 0) < PER_CARD_REVEALS_PROTOCOL) return null;
  if (snapshot.stage !== 'complete' || !snapshot.won_without_showdown || snapshot.winners?.length !== 1) return null;
  const winnerId = snapshot.winners[0];
  const requests = snapshot.winner_cards_requests ?? [];
  if (winnerId === viewer) return requests.length ? {winnerId, mode: 'prompt', requests} : null;

  // The server only ever lists this viewer's own request to them.
  const own = requests.find(request => request.requester_id === viewer);
  if (own) return {winnerId, mode: 'waiting', request: own};
  const winner = snapshot.seats.find(seat => seat.player_id === winnerId);
  const viewerDealtIn = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  const alreadyShown = winner?.hole_cards?.some(card => card && card !== 'back');
  // A refusal (or a timeout) closes the hand: the cards stop being a button
  // so the winner cannot be asked again until the next deal.
  if (!winner?.dealt_in || !viewerDealtIn || alreadyShown || snapshot.winner_cards_closed) return null;
  return {winnerId, mode: 'offer'};
}
