import type {BetPresetMode} from '@/lib/api/player';

/** Stride multiplier for a "faster" bet step (ctrl+arrow). Callers scale `step`
 * by it; the hold-repeat ramp scales the same way. */
export const FAST_STEP_STRIDE = 3;

/** Snap to the table's raise increment, then clamp into `[min, max]`.
 *
 * The one place that turns "a number a player asked for" into "a number the
 * server will accept": the quick-bet row snaps its fractions with it, and the
 * typed amount field applies it once on blur/Enter (never per keystroke — see
 * `checkBetInput`). Clamping last is deliberate: `min` and `max` are the
 * server's own figures and need not sit on the increment grid, so snapping
 * after them could push a value back outside the legal range. */
export function clampSnapRaise(value: number, min: number, max: number, step: number): number {
  const safeStep = Math.max(1, step);
  return Math.min(max, Math.max(min, Math.round(value / safeStep) * safeStep));
}

export function betShortcutAmount(
  key: string,
  current: number,
  min: number,
  max: number,
  step: number,
  halfPot?: number
) {
  if (key === 'a' || key === 'ArrowUp') return max;
  if (key === 'h') return halfPot ?? min;
  if (key === 'ArrowDown') return min;
  if (key === 'ArrowLeft' || key === 'ArrowRight') {
    const delta = step * (key === 'ArrowRight' ? 1 : -1);
    return Math.min(max, Math.max(min, current + delta));
  }
  return undefined;
}

/** The stage-aware quick-bet row (#341). Which set shows is a player
 * preference (`bet_preset_mode`): 'bb' is always big-blind multiples, 'pot' is
 * always pot fractions, and 'mixed' — the default — follows how the street is
 * actually talked about, opening in big blinds pre-flop and sizing against the
 * pot from the flop on.
 *
 * Pot fractions are the server's own raise-to figures (`legal_actions`), never
 * `pot * fraction`: a pot-size raise has to absorb the outstanding call, and
 * re-deriving it here would paint a number the server would reject. The big
 * blind set needs no help — a "3BB open" is literally a raise-to of three big
 * blinds.
 *
 * Every value is snapped to `raiseStep` and clamped into `[minRaise, maxRaise]`
 * (so nothing below the minimum is ever offered), then collapsed duplicates are
 * dropped keeping the LAST holder of each value: a short stack squeezes several
 * fractions onto the same all-in total, and the button left standing must be
 * the one that says All in, not a ⅓ that silently means "everything". */
export function stageBetPresets({mode, stage, bigBlind, serverPresets, minRaise, maxRaise, raiseStep}: {
  mode: BetPresetMode;
  stage: string;
  bigBlind: number;
  serverPresets: { label: string; value: number }[];
  minRaise: number;
  maxRaise: number;
  raiseStep: number;
}): { label: string; value: number }[] {
  if (maxRaise < minRaise) return [];
  const clamp = (value: number) => clampSnapRaise(value, minRaise, maxRaise, raiseStep);
  const useBigBlind = mode === 'bb' || (mode === 'mixed' && stage === 'pre_flop');
  const base = useBigBlind
    ? BB_PRESETS.map(({label, multiple}) => ({label, value: clamp(multiple * Math.max(1, bigBlind))}))
    // A bigger fraction must never cost less than a smaller one. An older
    // server (or the dev mock) omits some of the raise-to fields, and
    // `actionState` fills the gap with `minRaise` — which would have painted
    // "2/3" for less money than "1/2". Keep the set strictly increasing and
    // the missing fraction simply does not appear.
    : POT_PRESETS.reduce<{ label: string; value: number }[]>((kept, {label, serverLabel}) => {
      const server = serverPresets.find(preset => preset.label === serverLabel)?.value;
      if (server == null) return kept;
      const value = clamp(server);
      const previous = kept.at(-1);
      return previous && value <= previous.value ? kept : [...kept, {label, value}];
    }, []);
  const all = [...base, {label: ALL_IN_PRESET_LABEL, value: maxRaise}];
  return all.filter((preset, index) => !all.slice(index + 1).some(later => later.value === preset.value));
}

export const ALL_IN_PRESET_LABEL = 'All in';
const BB_PRESETS = [{label: 'BB', multiple: 1}, {label: '2BB', multiple: 2}, {label: '3BB', multiple: 3}];
const POT_PRESETS = [
  {label: '1/3', serverLabel: '⅓ pote'},
  {label: '1/2', serverLabel: '½ pote'},
  {label: '2/3', serverLabel: '⅔ pote'}
];
