import type {SeatView, TableSnapshot} from '@/lib/api/table';
import type {HandOutcomeState} from '@/components/table/HandOutcome';
import {bestFiveCardHand} from '@/lib/pokerRules';

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

/** The pot the server weighs when it records "maior pote de hoje": the sum of
 *  the contested layers' payouts, refund layers excluded (uncalled excess
 *  returned to its own bettor was never won). Mirrors `highlights.Store`'s
 *  `RecordHand`, which only overwrites today's row when this number beats the
 *  one on record — so a client that knows it can tell whether re-reading the
 *  highlight after a hand could possibly return anything new.
 *  `undefined` when the frame carries no `pot_results` and the amount is
 *  therefore unknowable here. */
export function highlightPot(snapshot: TableSnapshot): number | undefined {
  if (!snapshot.pot_results?.length) return undefined;
  return snapshot.pot_results.reduce((total, pot) => pot.refund ? total : total + pot.payout_amount, 0);
}

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

/** Every pot the viewer had a stake in, in the order the server resolved
 * them, so a 'mixed' result (won one pot, lost another) can show each pot's
 * actual outcome instead of collapsing a multi-pot showdown into a single
 * winner/loser pair. */
export function contestedPots(snapshot: TableSnapshot, viewer: string) {
  return (snapshot.pot_results ?? [])
    .filter(pot => !pot.refund && pot.winner_player_ids.length > 0 && pot.eligible_player_ids.includes(viewer))
    .map(pot => {
      const won = pot.winner_player_ids.includes(viewer);
      const winnerSeat = won ? undefined : snapshot.seats.find(seat => pot.winner_player_ids.includes(seat.player_id));
      return {won, winnerSeat};
    });
}

/** The other player(s) sharing a pot the viewer tied for, so a 2-way or
 * 3+-way chop can name every hand in the split instead of only the
 * viewer's own combination. */
export function tiedWinners(snapshot: TableSnapshot, viewer: string) {
  const otherIds = new Set((snapshot.pot_results ?? [])
    .filter(pot => !pot.refund && pot.winner_player_ids.length > 1 && pot.winner_player_ids.includes(viewer))
    .flatMap(pot => pot.winner_player_ids.filter(id => id !== viewer)));
  return snapshot.seats.filter(seat => otherIds.has(seat.player_id));
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

/** The two hole cards of a seat, but only when both are actually known — the
 * server masks anything unseen as the literal `"back"`, which must never be
 * reconstructed into a hand. */
function knownHoleCards(seat?: SeatView) {
  return seat?.hole_cards?.length === 2 &&
  seat.hole_cards.every(card => card.toLowerCase() !== 'back') ? seat.hole_cards : undefined;
}

/** The seat's hole cards reduced to the actual best five-card combination once
 * the board is complete — a bare pair of hole cards doesn't show what the
 * player won with when the winning combination uses the board too.
 * Presentation only: `bestFiveCardHand` orders the cards shown in the banner
 * and never decides who won (that stays server-authoritative). */
function resolvedHand(snapshot: TableSnapshot, seat?: SeatView) {
  const hole = knownHoleCards(seat);
  return hole && snapshot.board.length === 5 ? bestFiveCardHand([...hole, ...snapshot.board]) : hole;
}

/** Assembles the whole showdown banner for one resolved snapshot: the
 * win/lose/tie/mixed/fold reading, the hands worth naming beside it, the
 * per-pot detail a mixed result needs, and the viewer's chip delta.
 *
 * `rememberedStart` is the pre-blind stack remembered from the earliest live
 * frame of this hand, used only while API instances still predate protocol
 * v3's `stack_at_hand_start`. Returns null when the viewer was not dealt into
 * this hand — seat state does not prove participation, since a player may
 * become active mid-hand and only be eligible for the next deal. */
export function buildHandOutcome(snapshot: TableSnapshot, viewer: string,
                                 rememberedStart?: {handID: string; stack: number} | null,
                                 key = 0): HandOutcomeState | null {
  const seat = snapshot.seats.find(item => item.player_id === viewer);
  if (!shouldShowOutcome(seat)) return null;
  // Membership in `winners`, not a truthy payout, decides win/lose: an
  // uncalled all-in's excess or an orphaned side-pot refund also shows up in
  // `payouts` without being an actual win.
  const kind = tableOutcomeKind(snapshot, viewer);
  // The banner names one rival hand as the point of comparison when the viewer
  // lost at least one eligible pot. Only seats that reached showdown carry
  // hand_category, so this stays undefined (and the banner falls back to the
  // plain category chip) whenever the hand ended without one.
  const opponentCategory = (kind === 'lose' || kind === 'mixed') ?
    relevantWinner(snapshot, viewer)?.hand_category : undefined;
  // The hand that actually won this pot: the viewer's own on a win (always
  // known), or the first winning rival's on a loss, but only when a showdown
  // actually revealed them.
  const winnerSeat = kind === 'lose' ? relevantWinner(snapshot, viewer) : seat;
  const winnerHole = knownHoleCards(winnerSeat);
  // The best other hand the viewer actually beat, so a win can also read
  // "same combination, the kicker decided".
  const beatenSeat = kind === 'win' ? relevantRunnerUp(snapshot, viewer) : undefined;
  const viewerHole = knownHoleCards(seat);
  // A 'mixed' result means the viewer won at least one contested pot and lost
  // at least one other; per-pot detail is required because the pot the viewer
  // lost may have a different winner (and hand) than the pot they won.
  const pots = kind === 'mixed' ? contestedPots(snapshot, viewer).map(potOutcome => potOutcome.won
    ? {won: true as const}
    : {
      won: false as const,
      winnerName: potOutcome.winnerSeat?.name,
      category: potOutcome.winnerSeat?.hand_category,
      winningCards: resolvedHand(snapshot, potOutcome.winnerSeat)
    }) : undefined;
  // A tie means every contested pot the viewer won was split; name the other
  // hand(s) in that split (2-way or 3+-way chop).
  const tiedWith = kind === 'tie' ? tiedWinners(snapshot, viewer).map(tiedSeat => ({
    name: tiedSeat.name, cards: resolvedHand(snapshot, tiedSeat)
  })) : undefined;
  const breakdown = playerPotBreakdown(snapshot, viewer);
  // Folding is not a loss in the sense of having contested the pot. It gets
  // its own banner, only naming a rival hand when the board actually ran to a
  // showdown that revealed one, and only claiming "poderia ter ganhado" when
  // the folded hand would truly have beaten what got shown. That comparison
  // is server-authoritative (hand_score), never the local evaluator.
  const folded = seat.state === 'folded';
  const winnerWasPubliclyRevealed = Boolean(winnerHole &&
    winnerSeat?.hole_cards_revealed?.length === 2 &&
    winnerSeat.hole_cards_revealed.every(Boolean));
  return {
    key,
    kind: folded ? 'fold' : kind,
    couldHaveWon: folded && winnerWasPubliclyRevealed &&
    seat.hand_score != null && winnerSeat?.hand_score != null ?
      seat.hand_score > winnerSeat.hand_score : undefined,
    handCategory: seat.hand_category,
    opponentCategory,
    winningCards: resolvedHand(snapshot, winnerSeat),
    winningHoleCards: winnerHole,
    viewerCards: resolvedHand(snapshot, seat),
    viewerHoleCards: viewerHole,
    beatenCards: resolvedHand(snapshot, beatenSeat),
    beatenCategory: beatenSeat?.hand_category,
    pots,
    tiedWith,
    runItTwice: Boolean(snapshot.board_two?.length),
    resolvedPots: (snapshot.pot_results ?? []).map(pot => ({
      amount: pot.amount,
      payoutAmount: pot.payout_amount,
      viewerPayout: pot.payouts?.[viewer],
      winnerNames: pot.winner_player_ids.map(playerId =>
        snapshot.seats.find(item => item.player_id === playerId)?.name)
        .filter((name): name is string => Boolean(name)),
      wonByViewer: pot.winner_player_ids.includes(viewer),
      viewerEligible: pot.eligible_player_ids.includes(viewer),
      split: pot.winner_player_ids.length > 1,
      refund: Boolean(pot.refund),
      runout: pot.runout
    })),
    winnerName: winnerSeat?.name,
    stackBefore: seat.stack_at_hand_start ??
      (rememberedStart && rememberedStart.handID === snapshot.hand_id ? rememberedStart.stack : undefined),
    stackAfter: seat.stack,
    wonAmount: breakdown.won,
    refundAmount: breakdown.refund
  };
}
