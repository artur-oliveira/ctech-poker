import {act, renderHook} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {achievementDescription, achievementExample, achievementLabel, achievementWalletMode} from './achievements';
import {achievementProgress} from './api/achievements';
import {cardLabel, cardPath} from './cards';
import {chipTier} from './chips';
import {isTableReaction, TABLE_REACTIONS} from './reactions';
import {useTablePreferences} from './tablePreferences';
import {cn, initials, playerName, rotateSeats} from './utils';

describe('shared presentation contracts', () => {
  test('achievement metadata covers known, category and unknown keys', () => {
    expect(achievementLabel('wins')).toBe('Vitórias');
    expect(achievementLabel('win_category_full_house')).toBe('Full house');
    expect(achievementLabel('new_backend_key')).toBe('new backend key');
    expect(achievementDescription('bluff')).toMatch(/showdown/);
    expect(achievementDescription('win_category_pair')).toMatch(/com par/i);
    expect(achievementDescription('unknown')).toBe('');
    expect(achievementExample('all_in')).toHaveLength(2);
    expect(achievementExample('win_category_straight_flush')).toHaveLength(5);
    expect(achievementExample('unknown')).toEqual([]);
  });

  // Mirrors api/internal/achievements/catalog.go. The point of duplicating the
  // keys here is to fail loudly the next time the backend grows the catalog and
  // the copy is not written, instead of shipping a raw snake_case key on screen.
  const BACKEND_KEYS = [
    'wins', 'hands_played', 'comeback', 'bluff', 'survivor', 'looser', 'fallen_king', 'almost_winner',
    'tied', 'bad_beat', 'cooler', 'cracked_aces', 'giant_slayer', 'showdown_warrior', 'all_in',
    'real_money_earned', 'sandbox_chips_earned', 'won_with_pocket_pair', 'won_full_table', 'won_heads_up',
    'lost_straight_flush_to_royal', 'first_hand_allin_win', 'beat_pocket_aces', 'beat_trips_or_better',
    'three_bet_won_no_showdown', 'folded_streak', 'four_to_royal_missed', 'four_to_straight_flush_missed',
    'paid_river_draw_missed', 'lost_river_after_leading_turn', 'won_runner_runner', 'won_with_nuts',
    'same_pocket_pair_streak',
    ...['high_card', 'pair', 'two_pair', 'three_of_a_kind', 'straight', 'flush', 'full_house',
      'four_of_a_kind', 'straight_flush', 'royal_flush'].map(c => `win_category_${c}`)
  ];

  test.each(BACKEND_KEYS)('%s has a written label and description', key => {
    expect(achievementLabel(key)).not.toBe(key.replaceAll('_', ' '));
    expect(achievementDescription(key)).not.toBe('');
  });

  test('the earned counters are scoped to the wallet that can produce them', () => {
    expect(achievementWalletMode('real_money_earned')).toBe('real');
    expect(achievementWalletMode('sandbox_chips_earned')).toBe('sandbox');
    expect(achievementWalletMode('wins')).toBeUndefined();
  });

  test('achievement progress is independent of tier order and detects max', () => {
    const tiers = [{stars: 3, threshold: 10}, {stars: 1, threshold: 1}, {stars: 2, threshold: 5}];
    expect(achievementProgress(tiers, 6)).toEqual({
      count: 6, starsFilled: 2, nextTier: {stars: 3, threshold: 10}, maxed: false,
    });
    expect(achievementProgress(tiers, 20)).toMatchObject({starsFilled: 3, nextTier: null, maxed: true});
  });
  
  test('card, chip and reaction helpers tolerate wire variants', () => {
    expect(cardPath('As')).toContain('/spade-ace.svg');
    expect(cardPath('back')).toContain('card-back');
    expect(cardLabel('TH')).toBe('dez de copas');
    expect(cardLabel('back')).toBe('carta desconhecida');
    expect(chipTier(0, 50)).toBe(0);
    expect(chipTier(10_000, 50)).toBe(5);
    expect(Object.keys(TABLE_REACTIONS).every(isTableReaction)).toBe(true);
    expect(isTableReaction('not-allowed')).toBe(false);
  });
  
  test('name, initials, class and seat rotation helpers handle edge cases', () => {
    expect(playerName('me', 'me', 'Ana')).toBe('Você');
    expect(playerName('other', 'me', 'Bia')).toBe('Bia');
    expect(playerName('unknown')).toBe('Visitante');
    expect(initials('Ana Maria Silva')).toBe('AS');
    expect(initials('')).toBe('?');
    expect(cn('px-2', false && 'hidden', 'px-4')).toBe('px-4');
    const seats = [{player_id: 'a'}, {player_id: 'b'}, {player_id: 'c'}];
    expect(rotateSeats(seats, 'b').map(s => s.player_id)).toEqual(['b', 'c', 'a']);
    expect(rotateSeats(seats, 'missing')).toBe(seats);
  });
  
  test('table preferences persist only normalized values', () => {
    const {result} = renderHook(() => useTablePreferences());
    expect(result.current.preferences.theme).toBe('classic');
    act(() => result.current.update({theme: 'ocean', dealerVoice: true, realityCheckMinutes: 30}));
    expect(result.current.preferences).toMatchObject({theme: 'ocean', dealerVoice: true, realityCheckMinutes: 30});
    
    act(() => {
      localStorage.setItem('ctech-poker:table-preferences:v1', '{"theme":"invalid","realityCheckMinutes":13}');
      window.dispatchEvent(new Event('ctech-poker:table-preferences'));
    });
    expect(result.current.preferences).toMatchObject({theme: 'classic', realityCheckMinutes: 60});
  });
});
