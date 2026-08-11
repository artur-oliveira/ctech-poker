import type {SeatView, TableSnapshot} from '@/lib/api/table';

export type TableOutcomeKind = 'win' | 'lose' | 'tie' | 'mixed';

export type PlayerPotBreakdown = {
  credit: number;
  won: number;
  refund: number;
};

export type WinnerStanding = {
  playerId: string;
  amount: number;
  place?: number;
  tied: boolean;
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

/** The mirror of relevantWinner for a viewer who won: the best-scoring other
 * player eligible for a pot the viewer took, so a win can also explain
 * "same combination, the kicker decided" instead of only ever showing that
 * note to the loser. Undefined whenever no other eligible seat had its cards
 * revealed (e.g. everyone else folded, or an older protocol instance never
 * sent hand_score). */
export function relevantRunnerUp(snapshot: TableSnapshot, viewer: string) {
  const runnerUpIds = new Set(snapshot.pot_results?.filter(pot =>
    !pot.refund && pot.winner_player_ids.includes(viewer))
    .flatMap(pot => pot.eligible_player_ids.filter(id => id !== viewer)) ?? []);
  return snapshot.seats
    .filter(seat => runnerUpIds.has(seat.player_id) && seat.hand_score != null &&
      seat.hole_cards_revealed?.length === 2 && seat.hole_cards_revealed.every(Boolean))
    .sort((a, b) => (b.hand_score ?? 0) - (a.hand_score ?? 0))[0];
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

/** Display standings are based on contested chips won, not the number of
 * winner IDs in the snapshot. Multiple winners of one pot are a tie; winners
 * of different pot layers receive an ordered place by their total award. */
export function winnerStandings(snapshot: TableSnapshot): WinnerStanding[] {
  const ids = snapshot.pot_results?.length
    ? [...new Set(snapshot.pot_results.filter(pot => !pot.refund)
      .flatMap(pot => pot.winner_player_ids))]
    : [...new Set(snapshot.winners || [])];
  const splitWinners = new Set(snapshot.pot_results?.filter(pot => !pot.refund && pot.winner_player_ids.length > 1)
    .flatMap(pot => pot.winner_player_ids) || []);
  const standings = ids.map(playerId => ({
    playerId,
    amount: playerPotBreakdown(snapshot, playerId).won,
    tied: splitWinners.has(playerId)
  })).sort((a, b) => b.amount - a.amount);
  let previousAmount: number | undefined;
  let place = 0;
  return standings.map((standing, index) => {
    if (standing.amount !== previousAmount) place = index + 1;
    previousAmount = standing.amount;
    return {...standing, place: standing.tied ? undefined : place};
  });
}
