/**
 * Mock mode is exclusively a local-development facility. The NODE_ENV guard
 * prevents a public environment variable from enabling it in production.
 */
export const USE_MOCK =
  process.env.NODE_ENV !== 'production' &&
  process.env.NEXT_PUBLIC_MOCK_API === 'true';

export const MOCK_PLAYER_ID = 'mock_player_ana';

export type MockScenario =
  'full_hand'
  | 'full_hand_loss'
  | 'full_hand_tie'
  | 'all_in'
  | 'auto_fold'
  | 'waiting'
  | 'pre_flop'
  | 'flop'
  | 'turn'
  | 'river'
  | 'showdown'
  | 'side_pot'
  | 'run_it_twice'
  | 'reconnecting'
  | 'action_error'
  | 'timeout'
  | 'complete_loss'
  | 'complete_tie'
  | 'fold_win'
  | 'complete';
