import type {SeatView, TableSnapshot} from '@/lib/api/table';

export type TableOutcomeKind = 'win' | 'lose' | 'tie';

export function seatParticipated(seat?: SeatView) {
  if (!seat) return false;
  if (seat.dealt_in !== undefined) return seat.dealt_in;
  // Protocol-v1 fallback: the viewer's own real cards were only included
  // when they had actually been dealt into the hand.
  return seat.hole_cards?.length === 2 && seat.hole_cards.some(card => card.toLowerCase() !== 'back');
}

export function shouldShowOutcome(seat?: SeatView): seat is SeatView {
  return seatParticipated(seat) && seat?.state !== 'folded';
}

export function tableOutcomeKind(snapshot: TableSnapshot, viewer: string): TableOutcomeKind {
  if (snapshot.pot_results?.length) {
    const wonPots = snapshot.pot_results.filter(pot => pot.winner_player_ids.includes(viewer));
    if (!wonPots.length) return 'lose';
    return wonPots.some(pot => pot.winner_player_ids.length === 1) ? 'win' : 'tie';
  }
  if (!snapshot.winners?.includes(viewer)) return 'lose';
  return (snapshot.winners?.length ?? 0) > 1 ? 'tie' : 'win';
}

export function relevantWinner(snapshot: TableSnapshot, viewer: string) {
  const winnerIds = snapshot.pot_results?.filter(pot => pot.eligible_player_ids.includes(viewer))
    .flatMap(pot => pot.winner_player_ids) ?? [];
  return snapshot.seats.find(seat => winnerIds.includes(seat.player_id)) ??
    snapshot.seats.find(seat => snapshot.winners?.includes(seat.player_id));
}
