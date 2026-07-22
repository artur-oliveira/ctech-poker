'use client';
import {useEffect, useState} from 'react';
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

/** True when the key press belongs to a text field (chat input, etc.), never the raise slider. */
function isTypingTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && !!target.closest('input:not([type=range]), textarea, select, [contenteditable]');
}

function isPlainKey(event: KeyboardEvent) {
  return !event.metaKey && !event.ctrlKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

/** Raise control. Keyed by `actionKey` in the parent so the chosen amount
 * resets to the street minimum on every new decision without an effect. */
function RaiseControl({minRaise, maxRaise, raiseStep, pot, disabled, pending, onRaise}: {
  minRaise: number; maxRaise: number; raiseStep: number; pot: number; disabled: boolean; pending: boolean;
  onRaise: (amount: number) => void;
}) {
  const [amount, setAmount] = useState(minRaise);
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
    if (inactive) return () => {
    };

    function onKey(event: KeyboardEvent) {
      if (!isPlainKey(event) || event.key.toLowerCase() !== 'r') return;
      event.preventDefault();
      onRaise(safeAmount);
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  return <>
    <label className="bet-control" htmlFor="raise-amount">
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
            aria-describedby="action-context" onClick={() => onRaise(safeAmount)} className="raise">
      {pending ? <><LoaderCircle className="action-spinner"/> {actionLabel.raise}</> :
        <span>Aumentar <kbd aria-hidden="true">R</kbd></span>}
    </Button>
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

  useEffect(() => {
    if (unavailable) return () => {
    };
    const keyActions: Record<string, PokerAction> = {f: 'fold', c: 'check', p: 'call'};

    function onKey(event: KeyboardEvent) {
      if (!isPlainKey(event)) return () => {
      };
      const action = keyActions[event.key.toLowerCase()];
      if (!action || !available[action]) return () => {
      };
      event.preventDefault();
      onActAction(action);
      return () => {
      };
    }

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  return <div className="action-bar" role="group" aria-label="Ações da rodada" aria-busy={pending !== null}>
    <p id="action-context" className="action-context" aria-live="polite">{context}</p>
    <div className="action-choices" role="group" aria-label="Ações rápidas">
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
    </div>
    <RaiseControl key={actionKey} minRaise={minRaise} maxRaise={maxRaise} raiseStep={raiseStep} pot={pot}
                  disabled={unavailable || !available.raise} pending={pending === 'raise'}
                  onRaise={amount => onActAction('raise', amount)}/>
    {error && <div className="action-error" role="alert">
        <CircleAlert aria-hidden="true"/><p>{error.message}</p>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar aviso"
                onClick={onDismissErrorAction}><X/></Button>
    </div>}
  </div>;
}
