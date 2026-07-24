'use client';
import {useCallback, useEffect, useState} from 'react';
import {CircleAlert, LoaderCircle, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {PokerAction} from '@/lib/api/table';
import type {ActionError} from '@/lib/hooks/useTableRealtime';

export type ActionAvailability = Record<PokerAction, boolean>

type Props = {
  onActAction: (action: PokerAction, amount?: number) => boolean;
  available: ActionAvailability;
  callAmount: number;
  minRaise: number;
  maxRaise: number;
  raiseStep: number;
  pot: number;
  actionKey: string;
  isTurn: boolean;
  connected: boolean;
  pending: PokerAction | null;
  error: ActionError | null;
  onDismissErrorAction: () => void
}

const actionLabel: Record<PokerAction, string> = {
  fold: 'Desistindo…', check: 'Confirmando…', call: 'Pagando…', raise: 'Aumentando…'
};

// "Fold"/"Check" stay in English deliberately — BR players call them that at
// the table even in Portuguese play — while "Pagar"/"Aumentar" carry an
// amount, where a loanword reads worse. Confirmed intentional; don't
// "fix" this into Desistir/Passar without checking first.

/** True when the key press belongs to a text field (chat input, etc.), never the raise slider. */
function isTypingTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && !!target.closest('input:not([type=range]), textarea, select, [contenteditable]');
}

function isPlainKey(event: KeyboardEvent) {
  return !event.metaKey && !event.ctrlKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

/** Same as isPlainKey but allows ctrlKey through — used only by the arrow-key
 * bet-adjust shortcuts, where holding ctrl means "step faster". */
function isBetAdjustKey(event: KeyboardEvent) {
  return !event.metaKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

// Holding ctrl while nudging the bet with the arrow keys steps this many
// times faster.
const FAST_STEP_MULTIPLIER = 3;

// Handhelds (≤800px or short landscape — keep in sync with the matching CSS
// media tier) don't have room to show the preset/slider sizing UI at all
// times alongside Fold/Check/Pagar, so it stays collapsed until the player
// taps Aumentar once to reveal it; desktop keeps it always open (CSS ignores
// the collapsed class outside this query).
const COMPACT_QUERY = '(max-width: 800px), (max-height: 620px) and (orientation: landscape)';

/** Raise control. Keyed by `actionKey` in the parent so the chosen amount
 * resets to the street minimum on every new decision without an effect. */
function RaiseControl({minRaise, maxRaise, raiseStep, pot, disabled, pending, onRaise}: {
  minRaise: number; maxRaise: number; raiseStep: number; pot: number; disabled: boolean; pending: boolean;
  onRaise: (amount: number) => void;
}) {
  const [amount, setAmount] = useState(minRaise);
  const [expanded, setExpanded] = useState(false);
  const safeAmount = Math.min(maxRaise, Math.max(minRaise, amount));
  const inactive = disabled || maxRaise <= minRaise;
  const snap = (value: number) => Math.min(maxRaise, Math.max(minRaise, Math.round(value / raiseStep) * raiseStep));
  const presets = [
    {label: 'Mín', value: minRaise},
    {label: '½ pote', value: snap(pot / 2)},
    {label: 'Pote', value: snap(pot)},
    {label: 'Máx', value: maxRaise},
  ];

  useEffect(() => {
    if (inactive) return undefined;

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
          setAmount(Math.min(maxRaise, Math.max(minRaise, Math.round(pot / 2 / raiseStep) * raiseStep)));
          return;
        }
        if (key === 'a') {
          event.preventDefault();
          setAmount(maxRaise);
          onRaise(maxRaise);
          return;
        }
      }
      if (!isBetAdjustKey(event)) return;
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        setAmount(minRaise);
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        setAmount(maxRaise);
      } else if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault();
        const step = raiseStep * (event.ctrlKey ? FAST_STEP_MULTIPLIER : 1);
        const delta = event.key === 'ArrowRight' ? step : -step;
        setAmount(value => Math.min(maxRaise, Math.max(minRaise, value + delta)));
      }
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [inactive, safeAmount, onRaise, minRaise, maxRaise, raiseStep, pot]);

  function handleRaiseClick() {
    if (!expanded && window.matchMedia(COMPACT_QUERY).matches) {
      setExpanded(true);
      return;
    }
    onRaise(safeAmount);
  }

  return <>
    <label className={`bet-control${expanded ? '' : ' bet-control-collapsed'}`} htmlFor="raise-amount">
      <span className="sr-only">Valor total do aumento</span>
      <div className="bet-presets" role="group" aria-label="Valores rápidos de aumento">
        {presets.map(preset => <button key={preset.label} type="button" disabled={inactive}
                                       onClick={() => setAmount(preset.value)}>{preset.label}</button>)}
      </div>
      <Input id="raise-amount" aria-describedby="action-context" type="range"
             min={minRaise} max={maxRaise} step={raiseStep} value={safeAmount}
             disabled={inactive}
             onChange={event => setAmount(Number(event.target.value))}
             aria-valuetext={`${safeAmount.toLocaleString('pt-BR')} fichas`}/>
      <output id="raise-amount-output" htmlFor="raise-amount">{safeAmount.toLocaleString('pt-BR')}</output>
    </label>
    <Button type="button" disabled={inactive} aria-keyshortcuts="r"
            aria-describedby="action-context" onClick={handleRaiseClick}
            className={`raise${expanded ? '' : ' raise-collapsed'}`}>
      {pending ? <><LoaderCircle className="action-spinner"/> {actionLabel.raise}</> :
        <span>{expanded ? `Aumentar ${safeAmount.toLocaleString('pt-BR')}` : 'Aumentar'} <kbd aria-hidden="true">R</kbd></span>}
    </Button>
    {expanded && <Button type="button" variant="ghost" className="raise-cancel"
                         onClick={() => setExpanded(false)}>Cancelar</Button>}
  </>;
}

export function ActionBar({
                            onActAction,
                            available,
                            callAmount,
                            minRaise,
                            maxRaise,
                            raiseStep,
                            pot,
                            actionKey,
                            isTurn,
                            connected,
                            pending,
                            error,
                            onDismissErrorAction
                          }: Props) {
  const unavailable = !connected || !isTurn || pending !== null;
  const context = !connected ? 'Reconectando antes de liberar as ações…' : pending ? actionLabel[pending] : !isTurn ? 'Aguarde sua vez.' : 'Sua vez de agir.';
  const label = (action: PokerAction, idle: string, key?: string) => {
    if (pending === action) {
      return <><LoaderCircle className="action-spinner"/> {actionLabel[action]}</>;
    }
    return <span>{idle}{key && <kbd aria-hidden="true">{key}</kbd>}</span>;
  };
  const onRaise = useCallback((amount: number) => onActAction('raise', amount), [onActAction]);
  const canFold = available.fold, canCheck = available.check, canCall = available.call;
  // Nothing to do this street at all (waiting for players, folded, showdown/
  // complete) — collapse the choice row + raise slider instead of painting
  // the full disabled control surface a spectating player has no use for.
  const noLegalActions = !canFold && !canCheck && !canCall && !available.raise;

  useEffect(() => {
    if (unavailable) return undefined;
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
  }, [unavailable, canFold, canCheck, canCall, onActAction]);

  return <div className="action-bar" role="group" aria-label="Ações da rodada" aria-busy={pending !== null}>
    <p id="action-context" className="action-context" aria-live="polite">{context}</p>
    {!noLegalActions && <div className="action-choices" role="group" aria-label="Ações rápidas">
      <Button type="button" variant="outline" disabled={unavailable || !available.fold}
              aria-describedby="action-context" aria-keyshortcuts="f"
              onClick={() => onActAction('fold')}>{label('fold', 'Fold', 'F')}</Button>
      <Button type="button" variant="outline" disabled={unavailable || !available.check}
              aria-describedby="action-context" aria-keyshortcuts="c"
              onClick={() => onActAction('check')}>{label('check', 'Check', 'C')}</Button>
      <Button type="button" variant="outline" disabled={unavailable || !available.call}
              aria-describedby="action-context" aria-keyshortcuts="p"
              onClick={() => onActAction('call')}
              className="call">{label('call', callAmount > 0 ? `Pagar ${callAmount.toLocaleString('pt-BR')}` : 'Pagar', 'P')}</Button>
    </div>}
    {!noLegalActions && <RaiseControl key={actionKey} minRaise={minRaise} maxRaise={maxRaise} raiseStep={raiseStep}
                                      pot={pot} disabled={unavailable || !available.raise}
                                      pending={pending === 'raise'} onRaise={onRaise}/>}
    {error && <div className="action-error" role="alert">
        <CircleAlert aria-hidden="true"/><p>{error.message}</p>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar aviso"
                onClick={onDismissErrorAction}><X/></Button>
    </div>}
  </div>;
}
