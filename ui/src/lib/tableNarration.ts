import {cardLabel} from '@/lib/cards';
import {playSound} from '@/lib/sound';
import {playerName} from '@/lib/utils';
import type {TableSnapshot} from '@/lib/api/table';

/** What one snapshot transition says out loud, and what it sounds like.
 *
 * Both read only the previous and next frames, so they were the two pieces of
 * `useTableRealtime` that never needed any of its refs. Moved verbatim. */
const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'aguardando jogadores', pre_flop: 'pré-flop', flop: 'flop', turn: 'turn', river: 'river',
  showdown: 'showdown', complete: 'mão encerrada'
};

export function describeSnapshot(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  const nameOf = (id: string) => next.seats.find(seat => seat.player_id === id)?.name;
  const playerLabel = (id: string) => playerName(id, viewerId, nameOf(id));
  if (!previous) return `Mesa atualizada. ${STAGE_LABELS[next.stage] || next.stage}.`;
  const messages: string[] = [];
  if (next.stage !== previous.stage) messages.push(`Etapa: ${STAGE_LABELS[next.stage] || next.stage}`);
  if (next.board.length > previous.board.length) {
    const dealt = next.board.slice(previous.board.length).map(cardLabel).join(', ');
    messages.push(`${next.board.length === 3 ? 'Flop' : next.board.length === 4 ? 'Turn' : 'River'}: ${dealt}`);
  }
  const previousSeats = new Map(previous.seats.map(seat => [seat.player_id, seat]));
  const bettor = next.seats.find(seat => seat.contributed > (previousSeats.get(seat.player_id)?.contributed || 0));
  if (bettor) {
    const added = bettor.contributed - (previousSeats.get(bettor.player_id)?.contributed || 0);
    messages.push(`${playerLabel(bettor.player_id)} colocou ${added.toLocaleString('pt-BR')} fichas no pote`);
  }
  if (next.current_player_id && next.current_player_id !== previous.current_player_id) {
    messages.push(next.current_player_id === viewerId ? 'Sua vez de agir' : `Vez de ${playerLabel(next.current_player_id)}`);
  }
  const nextHasPayouts = next.payouts && Object.keys(next.payouts).length > 0;
  const prevHasPayouts = previous.payouts && Object.keys(previous.payouts).length > 0;
  if (nextHasPayouts && !prevHasPayouts) {
    if (next.pot_results?.length) {
      for (const pot of next.pot_results) {
        if (pot.refund) {
          messages.push(...Object.entries(pot.payouts || {})
            .filter(([, amount]) => amount > 0)
            .map(([playerId, amount]) =>
              `${playerLabel(playerId)} recebeu ${amount.toLocaleString('pt-BR')} fichas devolvidas`));
        } else if (pot.winner_player_ids.length === 1) {
          const winner = pot.winner_player_ids[0];
          const amount = pot.payouts?.[winner] ?? pot.payout_amount;
          messages.push(`${playerLabel(winner)} ganhou ${amount.toLocaleString('pt-BR')} fichas`);
        } else if (pot.winner_player_ids.length > 1) {
          messages.push(`${pot.winner_player_ids.map(playerLabel).join(' e ')} dividiram um pote de ${pot.payout_amount.toLocaleString('pt-BR')} fichas`);
        }
      }
    } else {
      // Compatibility with protocol v1, which only published aggregate
      // credits and could not distinguish a win from a refund precisely.
      messages.push(...Object.entries(next.payouts || {})
        .filter(([playerId, amount]) => amount > 0 && next.winners?.includes(playerId))
        .map(([playerId, amount]) =>
          `${playerLabel(playerId)} recebeu ${amount.toLocaleString('pt-BR')} fichas`));
    }
  }
  return messages.join('. ');
}

// Plays at most one sound per snapshot transition (never on every broadcast):
// each condition compares against the previous snapshot exactly like
// describeSnapshot does). Priority: a new board card beats an all-in beats a
// bet beats a fold-to-one reveal, since at most one usually fires per frame
// anyway.
export function playSoundForTransition(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  const scheduled: number[] = [];
  if (!previous) return scheduled;
  // Table is busy with a lot going on at once. The turn ring alone is easy
  // to miss, so this fires independently of (and can co-occur with) whatever
  // else this transition triggers below (a bet, a fold-to-one reveal, etc).
  if (viewerId && next.current_player_id === viewerId && previous.current_player_id !== viewerId) {
    playSound('your_turn');
  }
  if (next.board.length > previous.board.length) {
    const added = next.board.length - previous.board.length;
    // Flop deals 3 cards one at a time (Board/PlayingCard stagger reveal via
    // --deal-index; see .board-card .card-reveal-inner in globals.css): one
    // reveal sound per card, timed to match. Keep this in sync with that
    // animation-delay with a little gap (currently 360ms/index). Turn/river add a single card
    // with no stagger.
    for (let i = 0; i < added; i++) {
      if (i === 0) playSound('reveal');
      else scheduled.push(window.setTimeout(() => playSound('reveal'), i * 360));
    }
    return scheduled;
  }
  const previousSeats = new Map(previous.seats.map(seat => [seat.player_id, seat]));
  const wentAllIn = next.seats.some(seat => seat.state === 'all_in' && previousSeats.get(seat.player_id)?.state !== 'all_in');
  if (wentAllIn) {
    playSound('all_in');
    return scheduled;
  }
  const pot = previous.seats.reduce((n, seat) => n + seat.contributed, 0);
  const bettor = next.seats.find(seat => seat.contributed > (previousSeats.get(seat.player_id)?.contributed || 0));
  if (bettor) {
    const added = bettor.contributed - (previousSeats.get(bettor.player_id)?.contributed || 0);
    playSound(pot > 0 && added >= pot / 2 ? 'half_pot' : 'bet');
    return scheduled;
  }
  if (next.stage === 'complete' && previous.stage !== 'complete' && !next.won_without_showdown) playSound('reveal');
  return scheduled;
}
