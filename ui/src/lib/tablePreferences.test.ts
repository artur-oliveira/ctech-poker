import {act, renderHook} from '@testing-library/react';
import {beforeEach, describe, expect, test} from 'vitest';
import {PREMIUM_FELT_IDS, TABLE_THEMES, useTablePreferences} from './tablePreferences';

const STORAGE_KEY = 'ctech-poker:table-preferences:v1';

describe('PREMIUM_FELT_IDS', () => {
  test('marks every non-default felt theme as premium', () => {
    expect(PREMIUM_FELT_IDS).toEqual(new Set(['midnight', 'burgundy', 'ocean']));
    expect(PREMIUM_FELT_IDS.has('classic')).toBe(false);
  });

  test('every premium id resolves to a real TABLE_THEMES entry', () => {
    for (const id of PREMIUM_FELT_IDS) expect(TABLE_THEMES[id]).toBeDefined();
  });
});

describe('useTablePreferences', () => {
  beforeEach(() => localStorage.clear());

  test('defaults with no theme field on the preferences type', () => {
    const {result} = renderHook(() => useTablePreferences());
    expect(result.current.preferences).toEqual({
      dealerVoice: false, voiceCommands: false, realityCheckMinutes: 60, equityTrainer: false
    });
    expect(result.current.preferences).not.toHaveProperty('theme');
  });

  test('a pre-existing stored blob with a theme field round-trips harmlessly (extra key ignored)', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      theme: 'midnight', dealerVoice: true, voiceCommands: false, realityCheckMinutes: 90, equityTrainer: true
    }));
    const {result} = renderHook(() => useTablePreferences());
    expect(result.current.preferences).toEqual({
      dealerVoice: true, voiceCommands: false, realityCheckMinutes: 90, equityTrainer: true
    });
  });

  test('update persists only the known fields', () => {
    const {result} = renderHook(() => useTablePreferences());
    act(() => result.current.update({dealerVoice: true}));
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
    expect(stored).not.toHaveProperty('theme');
    expect(stored.dealerVoice).toBe(true);
  });
});
