import type {SeatView, TableSnapshot} from '@/lib/api/table';

export type TableOutcomeKind = 'win' | 'lose' | 'tie' | 'mixed';

export type PlayerPotBreakdown = {
  credit: number;
  won: number;
  refund: number;
};

export function seatParticipated(seat?: SeatView) {
  if (!seat) return false;
  if (seat.dealt_in !== undefined) return seat.dealt_in;
  // Protocol-v1 fallback: the viewer's own real cards were only included
  // when they had actually been dealt into the hand.
  return seat.hole_cards?.length === 2 && seat.hole_cards.some(card => card.toLowerCase() !== 'back');
}

export function shouldShowOutcome(seat?: SeatView): seat is SeatView {
  return seatParticipated(seat);
}

export function tableOutcomeKind(snapshot: TableSnapshot, viewer: string): TableOutcomeKind {
  if (snapshot.pot_results?.length) {
    const contested = snapshot.pot_results.filter(pot =>
      !pot.refund && pot.winner_player_ids.length > 0 && pot.eligible_player_ids.includes(viewer));
    const wonPots = contested.filter(pot => pot.winner_player_ids.includes(viewer));
    if (!wonPots.length) return 'lose';
    if (wonPots.length < contested.length) return 'mixed';
    return wonPots.some(pot => pot.winner_player_ids.length === 1) ? 'win' : 'tie';
  }
  if (!snapshot.winners?.includes(viewer)) return 'lose';
  return (snapshot.winners?.length ?? 0) > 1 ? 'tie' : 'win';
}

export function relevantWinner(snapshot: TableSnapshot, viewer: string) {
  const winnerIds = snapshot.pot_results?.filter(pot =>
    !pot.refund && pot.eligible_player_ids.includes(viewer) && !pot.winner_player_ids.includes(viewer))
    .flatMap(pot => pot.winner_player_ids) ?? [];
  return snapshot.seats.find(seat => winnerIds.includes(seat.player_id)) ??
    snapshot.seats.find(seat => snapshot.winners?.includes(seat.player_id));
}

function potCreditFor(pot: NonNullable<TableSnapshot['pot_results']>[number], playerId: string) {
  if (pot.payouts && playerId in pot.payouts) return pot.payouts[playerId] || 0;
  if (!pot.winner_player_ids.includes(playerId) || !pot.winner_player_ids.length) return 0;
  // Protocol-v2 fallback: it did not identify the odd-chip recipient, so this
  // is the closest safe estimate until every API instance publishes v3.
  return Math.floor(pot.payout_amount / pot.winner_player_ids.length);
}

export function playerPotBreakdown(snapshot: TableSnapshot, playerId: string): PlayerPotBreakdown {
  if (!snapshot.pot_results?.length) {
    const credit = snapshot.payouts?.[playerId] || 0;
    return snapshot.winners?.includes(playerId)
      ? {credit, won: credit, refund: 0}
      : {credit, won: 0, refund: credit};
  }
  let won = 0;
  let refund = 0;
  for (const pot of snapshot.pot_results) {
    const amount = potCreditFor(pot, playerId);
    if (pot.refund || pot.winner_player_ids.length === 0) refund += amount;
    else won += amount;
  }
  return {credit: won + refund, won, refund};
}
