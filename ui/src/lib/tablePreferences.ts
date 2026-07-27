'use client';
import {useCallback, useMemo, useSyncExternalStore} from 'react';

export type TableThemeId = 'classic' | 'midnight' | 'burgundy' | 'ocean';

export type TablePreferences = {
  theme: TableThemeId;
  dealerVoice: boolean;
  voiceCommands: boolean;
  realityCheckMinutes: number;
};

export const TABLE_THEMES: Record<TableThemeId, {label: string; colors: [string, string]}> = {
  classic: {label: 'Clássico', colors: ['#18765b', '#084b38']},
  midnight: {label: 'Meia-noite', colors: ['#244b65', '#102b3d']},
  burgundy: {label: 'Bordô', colors: ['#71323b', '#35151b']},
  ocean: {label: 'Oceano', colors: ['#14717a', '#073f49']}
};

const STORAGE_KEY = 'ctech-poker:table-preferences:v1';
const CHANGE_EVENT = 'ctech-poker:table-preferences';
const DEFAULTS: TablePreferences = {
  theme: 'classic', dealerVoice: false, voiceCommands: false, realityCheckMinutes: 60
};
const REALITY_INTERVALS = new Set([0, 30, 60, 90, 120]);

function normalize(value: unknown): TablePreferences {
  const input = value && typeof value === 'object' ? value as Partial<TablePreferences> : {};
  return {
    theme: input.theme && input.theme in TABLE_THEMES ? input.theme : DEFAULTS.theme,
    dealerVoice: input.dealerVoice === true,
    voiceCommands: input.voiceCommands === true,
    realityCheckMinutes: typeof input.realityCheckMinutes === 'number' &&
    REALITY_INTERVALS.has(input.realityCheckMinutes) ? input.realityCheckMinutes : DEFAULTS.realityCheckMinutes
  };
}

function readRaw() {
  return localStorage.getItem(STORAGE_KEY) || '';
}

function subscribe(callback: () => void) {
  const onStorage = (event: StorageEvent) => {
    if (event.key === STORAGE_KEY) callback();
  };
  window.addEventListener('storage', onStorage);
  window.addEventListener(CHANGE_EVENT, callback);
  return () => {
    window.removeEventListener('storage', onStorage);
    window.removeEventListener(CHANGE_EVENT, callback);
  };
}

export function useTablePreferences() {
  const raw = useSyncExternalStore(subscribe, readRaw, () => '');
  const preferences = useMemo(() => {
    if (!raw) return DEFAULTS;
    try {
      return normalize(JSON.parse(raw));
    } catch {
      return DEFAULTS;
    }
  }, [raw]);
  const update = useCallback((patch: Partial<TablePreferences>) => {
    const next = normalize({...preferences, ...patch});
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    window.dispatchEvent(new Event(CHANGE_EVENT));
  }, [preferences]);
  return {preferences, update};
}
