'use client';
import {
  type CSSProperties, type PointerEvent as ReactPointerEvent, type ReactNode,
  useCallback, useEffect, useRef, useState
} from 'react';
import {CircleAlert, Clock3, LoaderCircle, Minus, Plus, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {PokerAction} from '@/lib/api/table';
import type {ActionError} from '@/lib/hooks/useTableRealtime';
import {betShortcutAmount, clampSnapRaise, FAST_STEP_STRIDE, stageBetPresets} from '@/lib/betShortcuts';
import {betCommitNote, checkBetInput} from '@/lib/betInput';
import type {BetPresetMode} from '@/lib/api/player';
import {useChipExact, useChipFormat, useChipUnit, useSandboxChips} from '@/lib/chipFormat';
import {VoiceActionButton} from '@/components/table/VoiceActionButton';
import {type ActionPreselection, resolvePreselection} from '@/lib/actionPreselection';
import {useLiveNow} from '@/lib/hooks/useLiveNow';
import {isPlainKey, isTypingTarget} from '@/lib/utils';
import {useHoldRepeat} from '@/lib/hooks/useHoldRepeat';
import {useTurnHaptic} from '@/lib/hooks/useTurnHaptic';

export type ActionAvailability = Record<PokerAction, boolean>

type Props = {
  onActAction: (action: PokerAction, amount?: number) => boolean;
  available: ActionAvailability;
  callAmount: number;
  minRaise: number;
  maxRaise: number;
  raiseStep: number;
  effectiveStack: number;
  raisePresets: { label: string; value: number }[];
  actionKey: string;
  isTurn: boolean;
  connected: boolean;
  pending: PokerAction | null;
  error: ActionError | null;
  onDismissErrorAction: () => void
  canPreselect: boolean;
  supportsCallPreselection: boolean;
  selectionScope: string;
  preselection: ActionPreselection | null;
  preselectionAmount: number;
  prospectiveCallAmount: number;
  onPreselectAction: (selection: ActionPreselection | null, amount?: number) => boolean;
  actionDeadlineMs?: number;
  actionBaseDeadlineMs?: number;
  timeBankMs: number;
  voiceCommands: boolean;
  shortcutsEnabled: boolean;
  // Sizing inputs for the quick-bet row: which set the player asked for, and
  // the street/blind it is sized against. See `stageBetPresets`.
  betPresetMode: BetPresetMode;
  stage: string;
  bigBlind: number;
}

const actionLabel: Record<PokerAction, string> = {
  fold: 'Desistindo…', check: 'Confirmando…', call: 'Pagando…', raise: 'Aumentando…'
};

// "Fold"/"Check" stay in English deliberately: BR players call them that at
// the table even in Portuguese play, while "Pagar"/"Aumentar" carry an
// amount, where a loanword reads worse. Confirmed intentional; don't
// "fix" this into Desistir/Passar without checking first.

/** Same as isPlainKey but allows ctrlKey through. Used only by the arrow-key
 * bet-adjust shortcuts, where holding ctrl means "step faster". */
function isBetAdjustKey(event: KeyboardEvent) {
  return !event.metaKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

// Handhelds (≤800px or short landscape; keep in sync with the matching CSS
// media tier) don't have room to show the preset/slider sizing UI at all
// times alongside Fold/Check/Pagar, so it stays collapsed until the player
// taps Aumentar once to reveal it; desktop keeps it always open (CSS ignores
// the collapsed class outside this query).
const COMPACT_QUERY = '(max-width: 800px), (max-height: 620px) and (orientation: landscape)';

function BetStepButton({direction, disabled, onStep}: {
  direction: -1 | 1;
  disabled: boolean;
  onStep: (stride: number) => void;
}) {
  const hold = useHoldRepeat();

  function start(event: ReactPointerEvent<HTMLButtonElement>) {
    if (disabled || event.button !== 0) return;
    hold.start(onStep);
  }

  function click() {
    if (hold.consumeRepeated()) return;
    onStep(1);
  }

  const label = direction < 0 ? 'Menos fichas' : 'Mais fichas';
  const Icon = direction < 0 ? Minus : Plus;
  return (
    <button type="button" className="bet-step-button" aria-label={label} disabled={disabled}
            onPointerDown={start} onPointerUp={hold.stop} onPointerCancel={hold.stop}
            onPointerLeave={hold.stop} onClick={click}>
      <Icon aria-hidden="true"/>
    </button>
  );
}

/** The raise total, and the one place a player can type it.
 *
 * It was a read-only `<output>`: the `+`/`−` hold-repeat tops out at a stride
 * of 10x`raiseStep` and the range slider is desktop-only, so on a deep stack
 * there was no way to name an exact number at all.
 *
 * `type="text"`, never `type="number"`: a number input silently accepts `e`,
 * `+` and `-`, and reports `value === ''` for anything it considers invalid,
 * which leaves nothing to filter. Validation is a controlled-value revert —
 * one `onChange` path, so a keystroke, a paste, a drop and an IME commit all
 * go through `checkBetInput` and a rejected candidate simply never becomes
 * state. `minRaise`/`raiseStep` are NOT enforced here (see `checkBetInput`):
 * they land once, on blur or Enter, via `clampSnapRaise`. */
function BetAmountField({id, amount, minAmount, maxAmount, raiseStep, isAllIn, wasClamped, disabled, className,
                          onAmountAction}: {
  id: string;
  amount: number;
  minAmount: number;
  maxAmount: number;
  raiseStep: number;
  isAllIn: boolean;
  wasClamped: boolean;
  disabled: boolean;
  className: string;
  onAmountAction: (amount: number) => void;
}) {
  const progress = maxAmount > 0 ? Math.min(1, amount / maxAmount) : 0;
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  const sandbox = useSandboxChips();
  // null means "not being edited": the field then shows the same abbreviated
  // figure every other chip readout on the table shows. The moment it is
  // editable it holds exact, round-trippable digits instead.
  const [draft, setDraft] = useState<string | null>(null);
  const [rejected, setRejected] = useState('');
  // Escape blurs the field synchronously, before React has flushed the state
  // update that cleared the draft — so `commit` would still see (and commit)
  // the very draft Escape was abandoning. A ref is read at the same instant it
  // is written, which is the whole point here.
  const abandoned = useRef(false);
  // What the field held when editing started, so Escape can put it back: every
  // accepted keystroke has already published its amount to the slider and the
  // raise button, and "abandon" has to undo those too, not just the text.
  const enteredWith = useRef(amount);

  function commit() {
    if (abandoned.current) {
      abandoned.current = false;
      setDraft(null);
      setRejected('');
      return;
    }
    const parsed = draft === null || draft === '' ? minAmount : Number(draft.replace(',', '.'));
    const typed = Number.isFinite(parsed) ? parsed : minAmount;
    const settled = clampSnapRaise(typed, minAmount, maxAmount, raiseStep);
    onAmountAction(settled);
    setDraft(null);
    // `wasClamped` cannot carry this any more: commit snaps `amount` itself,
    // so amount === safeAmount by the time the note would have rendered.
    setRejected(betCommitNote(typed, settled, minAmount));
  }

  return (
    <span className={`${className}${isAllIn ? ' is-all-in' : ''}`}>
      <small>{isAllIn ? 'All In' : 'Total'}</small>
      <input id={id} className="bet-amount-input" type="text" disabled={disabled}
             inputMode={sandbox ? 'numeric' : 'decimal'} autoComplete="off" enterKeyHint="done"
             aria-label={`Valor total do aumento${unit ? ', em fichas' : ''}. Máximo ${exact(maxAmount)}`}
             aria-describedby="action-context"
             value={draft ?? chips(amount)}
             onFocus={event => {
               // Write the exact figure straight to the node before the state
               // update so `select()` selects THAT, not the abbreviation it is
               // replacing — typing then overwrites instead of appending.
               const exact = String(amount);
               enteredWith.current = amount;
               event.currentTarget.value = exact;
               event.currentTarget.select();
               setDraft(exact);
             }}
             onChange={event => {
               const check = checkBetInput(event.target.value, {maxRaise: maxAmount, allowFraction: !sandbox});
               if (!check.ok) {
                 setRejected(check.reason);
                 return;
               }
               setRejected('');
               setDraft(event.target.value);
               if (check.amount != null) onAmountAction(check.amount);
             }}
             onBlur={commit}
             onKeyDown={event => {
               if (event.key === 'Enter') {
                 event.preventDefault();
                 commit();
                 event.currentTarget.blur();
               } else if (event.key === 'Escape') {
                 abandoned.current = true;
                 onAmountAction(enteredWith.current);
                 setDraft(null);
                 setRejected('');
                 event.currentTarget.blur();
               }
             }}/>
      <span className="bet-commitment-meter" aria-hidden="true">
        <i style={{'--bet-progress': progress} as CSSProperties}/>
      </span>
      {/* A rejected keystroke that just vanishes reads as a broken field, so
          it says why. The clamp note is suppressed mid-edit: "ajustado ao
          mínimo" while someone is still typing the first digit of 250.000 is
          a lie about a value they have not finished naming. */}
      {rejected ? <small className="bet-clamped-note" role="status">{rejected}</small> :
        draft === null && wasClamped ? <small className="bet-clamped-note" role="status">
          {isAllIn ? 'ajustado ao máximo' : 'ajustado ao mínimo'}
        </small> : null}
    </span>
  );
}

function TimeBankStatus({isTurn, baseDeadline, actionDeadline, balance}: {
  isTurn: boolean;
  baseDeadline?: number;
  actionDeadline?: number;
  balance: number;
}) {
  const now = useLiveNow(Boolean(isTurn && actionDeadline));
  const bankActive = Boolean(isTurn && baseDeadline && actionDeadline && now >= baseDeadline);
  const remaining = bankActive && actionDeadline ? Math.max(0, actionDeadline - now) : Math.max(0, balance);
  const seconds = Math.ceil(remaining / 1000);
  return <span className={`time-bank-status${bankActive ? ' active' : ''}${!isTurn ? ' inactive' : ''}`} role="timer"
               aria-label={`${bankActive ? 'Time bank em uso' : 'Time bank disponível'}: ${seconds} segundos`}
               title="Reserva de decisão: recarrega 5 segundos por mão, até 30">
    <Clock3 aria-hidden="true"/>
    <span>{bankActive ? 'Time bank em uso' : isTurn ? 'Reserva pronta' : 'Sua reserva'} <b>{seconds}s</b></span>
  </span>;
}

function PreselectionControls({
                                canPreselect,
                                supportsCallPreselection,
                                isTurn,
                                connected,
                                pending,
                                available,
                                callAmount,
                                maxRaise,
                                prospectiveCallAmount,
                                selection,
                                selectionAmount,
                                onSelectAction,
                                onAct,
                                shortcutsEnabled
                              }: {
  canPreselect: boolean;
  supportsCallPreselection: boolean;
  isTurn: boolean;
  connected: boolean;
  pending: PokerAction | null;
  available: ActionAvailability;
  callAmount: number;
  maxRaise: number;
  prospectiveCallAmount: number;
  selection: ActionPreselection | null;
  selectionAmount: number;
  onSelectAction: (selection: ActionPreselection | null, amount?: number) => boolean;
  onAct: (action: PokerAction, amount?: number) => boolean;
  shortcutsEnabled: boolean;
}) {
  const executedRef = useRef<string | null>(null);
  useEffect(() => {
    if (!selection || !isTurn) {
      executedRef.current = null;
      return;
    }
    if (!connected || pending) return;
    const legal = (Object.keys(available) as PokerAction[]).filter(action => available[action]);
    const action = resolvePreselection(selection, legal, callAmount, selectionAmount);
    const executionKey = action ? `${selection}:${selectionAmount}:${callAmount}:${action}` : null;
    if (action && executedRef.current !== executionKey) {
      executedRef.current = executionKey;
      if (action === 'raise') onAct(action, maxRaise);
      else onAct(action);
    }
  }, [selection, selectionAmount, isTurn, connected, pending, available, callAmount, maxRaise, onAct]);

  const chips = useChipFormat();
  const exact = useChipExact();
  const hasFixedCall = supportsCallPreselection && prospectiveCallAmount > 0;
  // Shared by both the button's onClick and the keyboard shortcuts below, so
  // there is exactly one place that decides what selecting/deselecting a
  // value means.
  const toggle = useCallback((value: ActionPreselection, amount = 0) =>
      onSelectAction(selection === value ? null : value, selection === value ? 0 : amount),
    [selection, onSelectAction]);

  useEffect(() => {
    if (!canPreselect || !shortcutsEnabled) return undefined;

    function onKey(event: KeyboardEvent) {
      if (!isPlainKey(event)) return;
      const key = event.key.toLowerCase();
      if (key === 'x') {
        event.preventDefault();
        toggle('check_fold');
      } else if (key === 'f') {
        event.preventDefault();
        toggle('fold');
      } else if (key === 'c' && supportsCallPreselection) {
        event.preventDefault();
        if (hasFixedCall) {
          if (selection === 'call') toggle('call_any');
          else if (selection === 'call_any') onSelectAction(null, 0);
          else toggle('call', prospectiveCallAmount);
        } else {
          toggle('call_any');
        }
      } else if (key === 'a') {
        event.preventDefault();
        toggle('all_in');
      }
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [canPreselect, shortcutsEnabled, selection, supportsCallPreselection, hasFixedCall, prospectiveCallAmount,
    onSelectAction, toggle]);

  if (!canPreselect && !selection) return null;
  // `label` is what a sighted player reads, `name` what assistive tech
  // announces — they differ for the fixed-amount Call, whose figure is a
  // `.preselect-amount` the compact tier hides so all four options stay on one
  // row without shrinking a tap target (see .action-preselectors in app.css).
  const option = (value: ActionPreselection, label: ReactNode, name: string, description: string,
                  key?: string, amount = 0) =>
    <button type="button" className={selection === value ? 'selected' : ''}
            aria-pressed={selection === value} aria-label={name} title={description}
            disabled={!connected || pending !== null}
            onClick={() => toggle(value, amount)}>
      <span>{label}{key && shortcutsEnabled && <kbd aria-hidden="true">{key}</kbd>}<small>{description}</small></span>
    </button>;
  return <div className="action-preselectors" role="group" aria-label="Preparar próxima ação">
    <span>Próxima ação</span>
    {option('check_fold', <><span className="preselect-wide">Check / Fold</span>
      <span className="preselect-tight">C/F</span></>, 'Check / Fold',
      'Check se for grátis; caso contrário, fold', 'X')}
    {option('fold', <>Fold</>, 'Fold', 'Desistir quando chegar sua vez', 'F')}
    {supportsCallPreselection && hasFixedCall && option('call',
      <>Call <i className="preselect-amount">{chips(prospectiveCallAmount)}</i></>,
      `Call ${exact(prospectiveCallAmount)}`,
      'Pagar somente este valor; cancela se a aposta aumentar', 'C', prospectiveCallAmount)}
    {supportsCallPreselection && option('call_any', <><span className="preselect-wide">Call Any</span>
      <span className="preselect-tight">Any</span></>, 'Call Any',
      'Pagar qualquer valor quando chegar sua vez', hasFixedCall ? undefined : 'C')}
    {option('all_in', <>All In</>, 'All In', 'Apostar tudo quando chegar sua vez', 'A')}
  </div>;
}

/** Raise control. Keyed by `actionKey` in the parent so the chosen amount
 * resets to the street minimum on every new decision without an effect. */
function RaiseControl({
                        minRaise, maxRaise, raiseStep, presets, disabled, pending, onRaise, onExpandedChange,
                        shortcutsEnabled, betPresetMode, stage, bigBlind
                      }: {
  minRaise: number; maxRaise: number; raiseStep: number; disabled: boolean; pending: boolean;
  presets: { label: string; value: number }[];
  onRaise: (amount: number) => void;
  onExpandedChange: (expanded: boolean) => void;
  shortcutsEnabled: boolean;
  betPresetMode: BetPresetMode;
  stage: string;
  bigBlind: number;
}) {
  const [amount, setAmount] = useState(minRaise);
  const [expanded, setExpanded] = useState(false);
  const safeAmount = Math.min(maxRaise, Math.max(minRaise, amount));
  const inactive = disabled || maxRaise < minRaise;
  // Raising to the max is shoving the whole stack, so call it what it is
  // instead of a "Pay" label with a number that happens to equal the stack.
  const isAllIn = safeAmount >= maxRaise;
  const wasClamped = amount !== safeAmount;
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  // Already snapped and clamped by `stageBetPresets`, so a pick can never be
  // the silent clamp the old server-preset row had to signal.
  const quickPresets = stageBetPresets({
    mode: betPresetMode, stage, bigBlind, serverPresets: presets, minRaise, maxRaise, raiseStep
  });

  const hold = useHoldRepeat();
  const adjust = useCallback((direction: -1 | 1, multiplier: number) => {
    const key = direction > 0 ? 'ArrowRight' : 'ArrowLeft';
    setAmount(value => betShortcutAmount(key, value, minRaise, maxRaise, raiseStep * multiplier) ?? value);
  }, [maxRaise, minRaise, raiseStep]);

  useEffect(() => {
    if (inactive || !shortcutsEnabled) {
      hold.stop();
      return undefined;
    }

    function onKey(event: KeyboardEvent) {
      const key = event.key.toLowerCase();
      if (isPlainKey(event)) {
        if (key === 'r') {
          event.preventDefault();
          onRaise(safeAmount);
          return;
        }
        if (key === 'h') {
          event.preventDefault();
          setAmount(betShortcutAmount(key, safeAmount, minRaise, maxRaise, raiseStep,
            presets.find(preset => preset.label === '½ pote')?.value) ?? safeAmount);
          return;
        }
        if (key === 'a') {
          event.preventDefault();
          setAmount(betShortcutAmount(key, safeAmount, minRaise, maxRaise, raiseStep) ?? safeAmount);
          return;
        }
      }
      if (!isBetAdjustKey(event)) return;
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        setAmount(betShortcutAmount(event.key, safeAmount, minRaise, maxRaise, raiseStep) ?? safeAmount);
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        setAmount(betShortcutAmount(event.key, safeAmount, minRaise, maxRaise, raiseStep) ?? safeAmount);
      } else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault();
        // The first press steps once immediately; holding the key then hands
        // the cadence to useHoldRepeat, the same accelerating ramp the mobile
        // +/- buttons use, so a deep stack is reachable without spamming keys.
        const direction = event.key === 'ArrowRight' ? 1 : -1;
        const stride = event.ctrlKey ? FAST_STEP_STRIDE : 1;
        adjust(direction, stride);
        hold.start(multiplier => adjust(direction, multiplier * stride));
      }
    }

    function onKeyUp(event: KeyboardEvent) {
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') hold.stop();
    }

    window.addEventListener('keydown', onKey);
    window.addEventListener('keyup', onKeyUp);
    // A window blur (alt-tab, a focused iframe) swallows the keyup, which would
    // otherwise leave the amount climbing on its own.
    window.addEventListener('blur', hold.stop);
    // No hold.stop() here: this effect re-runs on every amount change, so
    // stopping would cancel the hold on its own first step. The hook clears its
    // timers on unmount, and keyup/blur/inactive end the hold otherwise.
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('keyup', onKeyUp);
      window.removeEventListener('blur', hold.stop);
    };
  }, [inactive, shortcutsEnabled, safeAmount, onRaise, minRaise, maxRaise, raiseStep, presets, adjust, hold]);

  function handleRaiseClick() {
    if (!expanded && window.matchMedia(COMPACT_QUERY).matches) {
      setExpanded(true);
      onExpandedChange(true);
      return;
    }
    onRaise(safeAmount);
  }

  return <>
    <label className={`bet-control${expanded ? '' : ' bet-control-collapsed'}`} htmlFor="raise-amount">
      <span className="sr-only">Valor total do aumento. Setas esquerda e direita ajustam; segure para acelerar</span>
      <div className="bet-presets" role="group" aria-label="Valores rápidos de aumento">
        {quickPresets.map(preset => <button key={preset.label} type="button" disabled={inactive}
                                            aria-label={`${preset.label}: aumentar para ${exact(preset.value)}`}
                                            onClick={() => setAmount(preset.value)}>{preset.label}</button>)}
      </div>
      <Input id="raise-amount" className="bet-range" aria-describedby="action-context" type="range"
             aria-keyshortcuts={shortcutsEnabled ? 'a h ArrowUp ArrowDown ArrowLeft ArrowRight' : undefined}
             min={minRaise} max={maxRaise} step={raiseStep} value={safeAmount}
             disabled={inactive}
             onChange={event => setAmount(Number(event.target.value))}
             aria-valuetext={`Total ${exact(safeAmount)}${unit}${isAllIn ? ', All In' : ''}`}/>
      <BetAmountField id="raise-amount-typed" className="bet-output bet-output-desktop" amount={safeAmount}
                      minAmount={minRaise} maxAmount={maxRaise} raiseStep={raiseStep} disabled={inactive}
                      isAllIn={isAllIn} wasClamped={wasClamped} onAmountAction={setAmount}/>
      <div className="bet-stepper" role="group" aria-label="Ajustar valor do aumento">
        <BetStepButton direction={-1} disabled={inactive} onStep={multiplier => adjust(-1, multiplier)}/>
        <BetAmountField id="raise-amount-typed-compact" className="bet-output bet-output-mobile" amount={safeAmount}
                        minAmount={minRaise} maxAmount={maxRaise} raiseStep={raiseStep} disabled={inactive}
                        isAllIn={isAllIn} wasClamped={wasClamped} onAmountAction={setAmount}/>
        <BetStepButton direction={1} disabled={inactive} onStep={multiplier => adjust(1, multiplier)}/>
      </div>
    </label>
    <Button type="button" disabled={inactive} aria-keyshortcuts={shortcutsEnabled ? 'r' : undefined}
            aria-describedby="action-context" onClick={handleRaiseClick}
            className={`raise${expanded ? '' : ' raise-collapsed'}`}>
      {pending ? <><LoaderCircle className="action-spinner"/> {isAllIn ? 'Indo All In…' : actionLabel.raise}</> :
        // "para" makes explicit this is a raise-to-total, not an amount added on top
        // of the current bet (unlike Pagar's amount above, which is additive). Same
        // Verb + Amount shape as Pagar otherwise read as the same kind of number.
        <span aria-label={expanded ? `${isAllIn ? 'All In' : 'Aumentar para'} ${exact(safeAmount)}` : undefined}>{expanded ? (isAllIn ? `All In ${chips(safeAmount)}` : `Aumentar para ${chips(safeAmount)}`) : (isAllIn ? 'All In' : 'Aumentar')}
          {shortcutsEnabled && <kbd aria-hidden="true">R</kbd>}</span>}
    </Button>
    {expanded && <Button type="button" variant="ghost" className="raise-cancel"
                         onClick={() => {
                           setExpanded(false);
                           onExpandedChange(false);
                         }}>Cancelar</Button>}
  </>;
}

export function ActionBar({
                            onActAction,
                            available,
                            callAmount,
                            minRaise,
                            maxRaise,
                            raiseStep,
                            effectiveStack,
                            raisePresets,
                            actionKey,
                            isTurn,
                            connected,
                            pending,
                            error,
                            onDismissErrorAction,
                            canPreselect,
                            supportsCallPreselection,
                            selectionScope,
                            preselection,
                            preselectionAmount,
                            prospectiveCallAmount,
                            onPreselectAction,
                            actionDeadlineMs,
                            actionBaseDeadlineMs,
                            timeBankMs,
                            voiceCommands,
                            shortcutsEnabled,
                            betPresetMode,
                            stage,
                            bigBlind
                          }: Props) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  // One short buzz when the turn opens, for the player who has looked away.
  useTurnHaptic(isTurn);
  const [raiseSizing, setRaiseSizing] = useState(false);
  const [raiseScope, setRaiseScope] = useState(actionKey);
  if (raiseScope !== actionKey) {
    setRaiseScope(actionKey);
    setRaiseSizing(false);
  }
  const legalActions = (Object.keys(available) as PokerAction[]).filter(action => available[action]);
  const preparedAction = selectionScope && preselection && isTurn ?
    resolvePreselection(preselection, legalActions, callAmount, preselectionAmount) : null;
  // Suppress the ordinary controls in the very first paint of the viewer's
  // turn, before the effect above submits the prepared action. If submission
  // is rejected, the visible error releases the controls for a manual choice.
  const executingPreparedAction = preparedAction !== null && error === null;
  const unavailable = !connected || !isTurn || pending !== null || executingPreparedAction;
  // The server still offers `raise` when the only legal raise-to band is a
  // sliver near all-in (an opponent shoved for less than a full raise). The
  // slider then pins to the stack and every preset collapses onto "All in" —
  // without a word, players read that as "raising is disabled". `raiseStep` is
  // the smallest legal increment, so a band narrower than one step has no room
  // for a raise that is not also a shove.
  const raiseBandCollapsed = available.raise && maxRaise - minRaise < raiseStep;
  const turnContext = effectiveStack > 0 ?
    `Sua vez de agir. Stack efetivo: ${exact(effectiveStack)}${unit}.` : 'Sua vez de agir.';
  const context = !connected ? 'Reconectando antes de liberar as ações…' : pending ? actionLabel[pending] :
    executingPreparedAction ? 'Executando sua ação preparada…' : !isTurn ? 'Aguarde sua vez.' :
      raiseBandCollapsed ? `${turnContext} Aumento mínimo é ${exact(minRaise)}, só resta ir all in.` :
        turnContext;
  const label = (action: PokerAction, idle: string, key?: string) => {
    if (pending === action) {
      return <><LoaderCircle className="action-spinner"/> {actionLabel[action]}</>;
    }
    return <span>{idle}{key && shortcutsEnabled && <kbd aria-hidden="true">{key}</kbd>}</span>;
  };
  const onRaise = useCallback((amount: number) => onActAction('raise', amount), [onActAction]);
  const canFold = available.fold, canCheck = available.check, canCall = available.call;
  // Nothing to do this street at all (waiting for players, folded, showdown/
  // complete), so collapse the choice row + raise slider instead of painting
  // the full disabled control surface a spectating player has no use for.
  const noLegalActions = !canFold && !canCheck && !canCall && !available.raise;

  useEffect(() => {
    if (unavailable || !shortcutsEnabled) return undefined;
    const keyActions: Record<string, PokerAction> = {f: 'fold', c: 'check', p: 'call'};
    const legal: Record<string, boolean> = {f: canFold, c: canCheck, p: canCall};

    function onKey(event: KeyboardEvent) {
      if (!isPlainKey(event)) return;
      const key = event.key.toLowerCase();
      const action = keyActions[key];
      if (!action || !legal[key]) return;
      event.preventDefault();
      onActAction(action);
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [unavailable, shortcutsEnabled, canFold, canCheck, canCall, onActAction]);

  return <div className={`action-bar${isTurn ? ' is-turn' : ''}${raiseSizing ? ' is-sizing' : ''}`} role="group"
              aria-label="Ações da rodada" aria-busy={pending !== null}>
    <div className="action-context-row">
      <p id="action-context" className="action-context" aria-live="polite">{context}</p>
      <TimeBankStatus isTurn={isTurn} baseDeadline={actionBaseDeadlineMs}
                      actionDeadline={actionDeadlineMs} balance={timeBankMs}/>
      <VoiceActionButton enabled={voiceCommands} disabled={unavailable}
                         available={available} minRaise={minRaise} maxRaise={maxRaise}
                         onAct={onActAction}/>
    </div>
    <PreselectionControls key={selectionScope} canPreselect={canPreselect}
                          supportsCallPreselection={supportsCallPreselection} isTurn={isTurn}
                          connected={connected} pending={pending} available={available}
                          callAmount={callAmount} maxRaise={maxRaise} prospectiveCallAmount={prospectiveCallAmount}
                          selection={preselection} selectionAmount={preselectionAmount}
                          onSelectAction={onPreselectAction} onAct={onActAction}
                          shortcutsEnabled={shortcutsEnabled}/>
    {!noLegalActions && !executingPreparedAction &&
        <div className="action-choices" role="group" aria-label="Ações rápidas">
            <Button type="button" variant="outline" disabled={unavailable || !available.fold}
                    aria-describedby="action-context" aria-keyshortcuts={shortcutsEnabled ? 'f' : undefined}
                    onClick={() => onActAction('fold')}>{label('fold', 'Fold', 'F')}</Button>
            <Button type="button" variant="outline" disabled={unavailable || !available.check}
                    aria-describedby="action-context" aria-keyshortcuts={shortcutsEnabled ? 'c' : undefined}
                    onClick={() => onActAction('check')}>{label('check', 'Check', 'C')}</Button>
            <Button type="button" variant="outline" disabled={unavailable || !available.call}
                    aria-describedby="action-context" aria-keyshortcuts={shortcutsEnabled ? 'p' : undefined}
                    onClick={() => onActAction('call')}
                    aria-label={callAmount > 0 ? `Pagar ${exact(callAmount)}` : undefined}
                    className="call">{label('call', callAmount > 0 ? `Pagar ${chips(callAmount)}` : 'Pagar', 'P')}</Button>
        </div>}
    {!noLegalActions && !executingPreparedAction &&
        <RaiseControl key={actionKey} minRaise={minRaise} maxRaise={maxRaise} raiseStep={raiseStep}
                      disabled={unavailable || !available.raise} presets={raisePresets}
                      pending={pending === 'raise'} onRaise={onRaise} onExpandedChange={setRaiseSizing}
                      shortcutsEnabled={shortcutsEnabled} betPresetMode={betPresetMode}
                      stage={stage} bigBlind={bigBlind}/>}
    {error && <div className="action-error" role="alert">
        <CircleAlert aria-hidden="true"/><p>{error.message}</p>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar aviso"
                onClick={onDismissErrorAction}><X/></Button>
    </div>}
  </div>;
}
