import assert from 'node:assert/strict';
import {test} from 'vitest';
import type {TableSnapshot} from './api/table.ts';
import {
  buildHandOutcome,
  contestedPots,
  playerPotBreakdown,
  relevantRunnerUp,
  seatParticipated,
  shouldShowOutcome,
  tableOutcomeKind,
  tiedWinners,
  winnerStandings
} from './tableOutcome.ts';

const snapshot: TableSnapshot = {
  stage: 'complete',
  board: [],
  seats: [
    {player_id: 'main', stack: 100, state: 'all_in', dealt_in: true, contributed: 50},
    {player_id: 'side', stack: 200, state: 'all_in', dealt_in: true, contributed: 100},
    {player_id: 'loser', stack: 0, state: 'sitting_out', dealt_in: true, contributed: 100}
  ],
  winners: ['main', 'side'],
  pot_results: [
    {
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side', 'loser'], winner_player_ids: ['main'], payouts: {main: 150}
    },
    {
      amount: 100, payout_amount: 100,
      eligible_player_ids: ['side', 'loser'], winner_player_ids: ['side'], payouts: {side: 100}
    }
  ]
};

test('different side-pot winners are never mislabeled as a tie', () => {
  assert.equal(tableOutcomeKind(snapshot, 'main'), 'win');
  assert.equal(tableOutcomeKind(snapshot, 'side'), 'mixed');
  assert.equal(tableOutcomeKind(snapshot, 'loser'), 'lose');
});

test('orders side-pot winners by chips won instead of presenting a tie', () => {
  assert.deepEqual(winnerStandings(snapshot), [
    {playerId: 'main', amount: 150, place: 1, tied: false},
    {playerId: 'side', amount: 100, place: 2, tied: false}
  ]);
});

test('only winners sharing the same pot are tied', () => {
  const tied = {
    ...snapshot,
    pot_results: [{
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side'], winner_player_ids: ['main', 'side']
    }]
  };
  assert.equal(tableOutcomeKind(tied, 'main'), 'tie');
  assert.equal(tableOutcomeKind(tied, 'side'), 'tie');
  assert.deepEqual(winnerStandings(tied).map(item => ({playerId: item.playerId, tied: item.tied})), [
    {playerId: 'main', tied: true}, {playerId: 'side', tied: true}
  ]);
});

test('winning one pot and losing another is a mixed result', () => {
  const mixed = {
    ...snapshot,
    pot_results: [
      snapshot.pot_results![0],
      {...snapshot.pot_results![1], eligible_player_ids: ['main', 'side', 'loser']}
    ]
  };
  assert.equal(tableOutcomeKind(mixed, 'main'), 'mixed');
});

test('contestedPots reports the actual winner of each pot in a mixed result, not one flattened rival', () => {
  const mixed = {
    ...snapshot,
    pot_results: [
      snapshot.pot_results![0],
      {...snapshot.pot_results![1], eligible_player_ids: ['main', 'side', 'loser']}
    ]
  };
  assert.deepEqual(
    contestedPots(mixed, 'main').map(pot => ({won: pot.won, winnerId: pot.winnerSeat?.player_id})),
    [{won: true, winnerId: undefined}, {won: false, winnerId: 'side'}]
  );
});

test('losing the contested pot while your own uncalled excess is refunded is still a clean loss, not mixed', () => {
  // The exact shape of an all-in that gets fully called for less than
  // shoved: one contested pot (you vs. the caller, you lose it) plus a
  // refund-only layer no one else ever staked (your own excess chips
  // coming back). `tableOutcomeKind` must not count that refund layer as a
  // pot you "won" — the player put no one else's money at risk in it.
  const shortCallLoss: TableSnapshot = {
    ...snapshot,
    winners: ['side'],
    pot_results: [
      {
        amount: 100, payout_amount: 100,
        eligible_player_ids: ['main', 'side'], winner_player_ids: ['side'], payouts: {side: 100}
      },
      {
        amount: 40, payout_amount: 40, eligible_player_ids: ['main'],
        winner_player_ids: [], payouts: {main: 40}, refund: true
      }
    ]
  };
  assert.equal(tableOutcomeKind(shortCallLoss, 'main'), 'lose');
});

test('tiedWinners names only the other players sharing a split pot, 2-way and 3+-way', () => {
  const twoWay = {
    ...snapshot,
    pot_results: [{
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side'], winner_player_ids: ['main', 'side']
    }]
  };
  assert.deepEqual(tiedWinners(twoWay, 'main').map(seat => seat.player_id), ['side']);

  const threeWay = {
    ...snapshot,
    pot_results: [{
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side', 'loser'], winner_player_ids: ['main', 'side', 'loser']
    }]
  };
  assert.deepEqual(tiedWinners(threeWay, 'main').map(seat => seat.player_id).sort(), ['loser', 'side']);
});

test('tiedWinners excludes a pot the viewer won outright and refund-only layers', () => {
  const soleWinner = {
    ...snapshot,
    pot_results: [snapshot.pot_results![0]]
  };
  assert.deepEqual(tiedWinners(soleWinner, 'main'), []);
});

test('relevantRunnerUp picks the best-scoring revealed opponent from a pot the viewer won', () => {
  const withScores: TableSnapshot = {
    ...snapshot,
    seats: [
      {
        player_id: 'main', stack: 100, state: 'all_in', dealt_in: true, contributed: 50,
        hand_score: 500, hole_cards_revealed: [true, true]
      },
      {
        player_id: 'side', stack: 200, state: 'all_in', dealt_in: true, contributed: 100,
        hand_score: 300, hole_cards_revealed: [true, true]
      },
      {
        player_id: 'loser', stack: 0, state: 'sitting_out', dealt_in: true, contributed: 100,
        hand_score: 100, hole_cards_revealed: [true, true]
      }
    ]
  };
  assert.equal(relevantRunnerUp(withScores, 'main')?.player_id, 'side');
});

test('relevantRunnerUp is undefined when the runner-up never revealed their cards', () => {
  const unrevealed: TableSnapshot = {
    ...snapshot,
    seats: snapshot.seats.map(seat => ({...seat, hand_score: seat.player_id === 'loser' ? 100 : undefined}))
  };
  assert.equal(relevantRunnerUp(unrevealed, 'main'), undefined);
});

test('contested winnings and refunds remain separate', () => {
  const withRefund = {
    ...snapshot,
    payouts: {main: 190},
    pot_results: [
      snapshot.pot_results![0],
      {
        amount: 40, payout_amount: 40, eligible_player_ids: ['main'],
        winner_player_ids: [], payouts: {main: 40}, refund: true
      }
    ]
  };
  assert.deepEqual(playerPotBreakdown(withRefund, 'main'), {credit: 190, won: 150, refund: 40});
});

test('a busted showdown participant still receives the outcome', () => {
  assert.equal(shouldShowOutcome(snapshot.seats[2]), true);
});

test('a folded participant still receives the outcome and reveal controls', () => {
  assert.equal(shouldShowOutcome({...snapshot.seats[0], state: 'folded'}), true);
});

test('protocol-v1 participation falls back to the viewer cards', () => {
  assert.equal(seatParticipated({
    player_id: 'old', stack: 10, state: 'active', contributed: 10, hole_cards: ['Ah', 'Kd']
  }), true);
  assert.equal(seatParticipated({
    player_id: 'waiting', stack: 10, state: 'active', contributed: 0
  }), false);
});

// --- buildHandOutcome: the whole banner, assembled from one resolved frame ---

const BOARD = ['Ah', 'Kd', '7c', '2s', '9h'];

function resolved(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'complete', hand_id: 'hand-1', board: BOARD, winners: ['hero'],
    seats: [
      {
        player_id: 'hero', name: 'Hero', stack: 300, state: 'active', dealt_in: true, contributed: 100,
        hole_cards: ['As', 'Ac'], hole_cards_revealed: [true, true], hand_category: 'three_of_a_kind',
        hand_score: 900, stack_at_hand_start: 200
      },
      {
        player_id: 'villain', name: 'Villain', stack: 0, state: 'active', dealt_in: true, contributed: 100,
        hole_cards: ['Kh', 'Kc'], hole_cards_revealed: [true, true], hand_category: 'two_pair',
        hand_score: 700
      }
    ],
    pot_results: [{
      amount: 200, payout_amount: 200,
      eligible_player_ids: ['hero', 'villain'], winner_player_ids: ['hero'], payouts: {hero: 200}
    }],
    ...overrides
  };
}

test('a win names the beaten hand and the viewer chip delta', () => {
  const outcome = buildHandOutcome(resolved(), 'hero', null, 7);
  assert.ok(outcome);
  assert.equal(outcome.key, 7);
  assert.equal(outcome.kind, 'win');
  assert.equal(outcome.winnerName, 'Hero');
  assert.equal(outcome.stackBefore, 200);
  assert.equal(outcome.stackAfter, 300);
  assert.equal(outcome.wonAmount, 200);
  assert.equal(outcome.beatenCategory, 'two_pair');
  // Presentation only: the hole cards are reduced against the complete board.
  assert.equal(outcome.viewerCards?.length, 5);
  assert.deepEqual(outcome.viewerHoleCards, ['As', 'Ac']);
  assert.equal(outcome.opponentCategory, undefined);
  assert.deepEqual(outcome.resolvedPots, [{
    amount: 200, payoutAmount: 200, viewerPayout: 200, winnerNames: ['Hero'],
    wonByViewer: true, viewerEligible: true, split: false, refund: false, runout: undefined
  }]);
});

test('a loss names the rival hand that took the pot', () => {
  const outcome = buildHandOutcome(resolved(), 'villain');
  assert.ok(outcome);
  assert.equal(outcome.kind, 'lose');
  assert.equal(outcome.opponentCategory, 'three_of_a_kind');
  assert.equal(outcome.winnerName, 'Hero');
  assert.deepEqual(outcome.winningHoleCards, ['As', 'Ac']);
  assert.equal(outcome.beatenCards, undefined);
  assert.equal(outcome.wonAmount, 0);
});

test('a mixed result carries per-pot detail, because each pot had its own winner', () => {
  const outcome = buildHandOutcome(snapshot, 'side');
  assert.ok(outcome);
  assert.equal(outcome.kind, 'mixed');
  assert.deepEqual(outcome.pots, [
    {won: false, winnerName: undefined, category: undefined, winningCards: undefined},
    {won: true}
  ]);
  assert.equal(outcome.tiedWith, undefined);
});

test('a tie names the other hands in the chop', () => {
  const outcome = buildHandOutcome(resolved({
    winners: ['hero', 'villain'],
    pot_results: [{
      amount: 200, payout_amount: 200, eligible_player_ids: ['hero', 'villain'],
      winner_player_ids: ['hero', 'villain'], payouts: {hero: 100, villain: 100}
    }]
  }), 'hero');
  assert.ok(outcome);
  assert.equal(outcome.kind, 'tie');
  assert.equal(outcome.tiedWith?.length, 1);
  assert.equal(outcome.tiedWith?.[0].name, 'Villain');
  assert.equal(outcome.tiedWith?.[0].cards?.length, 5);
  assert.equal(outcome.pots, undefined);
});

test('a fold that would have won says so, but only off the server hand scores', () => {
  const folded = resolved({
    winners: ['villain'],
    pot_results: [{
      amount: 200, payout_amount: 200, eligible_player_ids: ['hero', 'villain'],
      winner_player_ids: ['villain'], payouts: {villain: 200}
    }]
  });
  folded.seats[0] = {...folded.seats[0], state: 'folded'};
  const outcome = buildHandOutcome(folded, 'hero');
  assert.ok(outcome);
  assert.equal(outcome.kind, 'fold');
  // hero 900 > villain 700, and villain's cards were publicly revealed.
  assert.equal(outcome.couldHaveWon, true);

  // With the winner's cards never revealed there is nothing to compare against.
  const hidden = {...folded, seats: folded.seats.map(seat =>
    seat.player_id === 'villain' ? {...seat, hole_cards: ['back', 'back'], hole_cards_revealed: undefined} : seat)};
  assert.equal(buildHandOutcome(hidden, 'hero')?.couldHaveWon, undefined);
});

test('an unrevealed rival hand is never reconstructed into a shown combination', () => {
  const masked = resolved({
    winners: ['villain'],
    pot_results: [{
      amount: 200, payout_amount: 200, eligible_player_ids: ['hero', 'villain'],
      winner_player_ids: ['villain'], payouts: {villain: 200}
    }],
    seats: [
      resolved().seats[0],
      {...resolved().seats[1], hole_cards: ['back', 'back'], hole_cards_revealed: undefined}
    ]
  });
  const outcome = buildHandOutcome(masked, 'hero');
  assert.equal(outcome?.winningHoleCards, undefined);
  assert.equal(outcome?.winningCards, undefined);
});

test('an incomplete board leaves the hole cards as they are, unreduced', () => {
  const outcome = buildHandOutcome(resolved({board: ['Ah', 'Kd', '7c']}), 'hero');
  assert.deepEqual(outcome?.viewerCards, ['As', 'Ac']);
});

test('run-it-twice is reported so the banner can show both boards', () => {
  assert.equal(buildHandOutcome(resolved({board_two: BOARD}), 'hero')?.runItTwice, true);
  assert.equal(buildHandOutcome(resolved(), 'hero')?.runItTwice, false);
});

test('the remembered pre-blind stack only stands in for the hand it was captured on', () => {
  const legacy = resolved();
  legacy.seats[0] = {...legacy.seats[0], stack_at_hand_start: undefined};
  assert.equal(buildHandOutcome(legacy, 'hero', {handID: 'hand-1', stack: 175})?.stackBefore, 175);
  assert.equal(buildHandOutcome(legacy, 'hero', {handID: 'other-hand', stack: 175})?.stackBefore, undefined);
  assert.equal(buildHandOutcome(legacy, 'hero')?.stackBefore, undefined);
});

test('a viewer who was never dealt into the hand gets no banner at all', () => {
  const outcome = buildHandOutcome(resolved({
    seats: [...resolved().seats, {player_id: 'watcher', stack: 500, state: 'active', dealt_in: false, contributed: 0}]
  }), 'watcher');
  assert.equal(outcome, null);
  assert.equal(buildHandOutcome(resolved(), 'nobody'), null);
});
