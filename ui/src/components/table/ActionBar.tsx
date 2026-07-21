'use client'
import {useState} from 'react';
import {CircleAlert, LoaderCircle, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {PokerAction} from '@/lib/api/table';
import type {ActionError} from '@/lib/hooks/useTableRealtime';

export type ActionAvailability = Record<PokerAction, boolean>

type Props = {
  onAct: (action: PokerAction, amount?: number) => boolean;
  available: ActionAvailability;
  callAmount: number;
  minRaise: number;
  maxRaise: number;
  raiseStep: number;
  actionKey: string;
  isTurn: boolean;
  connected: boolean;
  pending: PokerAction | null;
  error: ActionError | null;
  onDismissError: () => void
}

const actionLabel: Record<PokerAction, string> = {
  fold: 'Desistindo…', check: 'Confirmando…', call: 'Pagando…', raise: 'Aumentando…'
}

/** Raise control. Keyed by `actionKey` in the parent so the chosen amount
 * resets to the street minimum on every new decision without an effect. */
function RaiseControl({minRaise, maxRaise, raiseStep, disabled, pending, onRaise}: {
  minRaise: number; maxRaise: number; raiseStep: number; disabled: boolean; pending: boolean;
  onRaise: (amount: number) => void;
}) {
  const [amount, setAmount] = useState(minRaise);
  const safeAmount = Math.min(maxRaise, Math.max(minRaise, amount));
  return <>
    <label className="bet-control" htmlFor="raise-amount">
      <span className="sr-only">Valor total do aumento</span>
      <Input id="raise-amount" aria-describedby="action-context" type="range"
             min={minRaise} max={maxRaise} step={raiseStep} value={safeAmount}
             disabled={disabled || maxRaise <= minRaise}
             onChange={event => setAmount(Number(event.target.value))}/>
      <output id="raise-amount-output" htmlFor="raise-amount">{safeAmount.toLocaleString('pt-BR')}</output>
    </label>
    <Button type="button" disabled={disabled || maxRaise <= minRaise}
            aria-describedby="action-context" onClick={() => onRaise(safeAmount)} className="raise">
      {pending ? <><LoaderCircle className="action-spinner"/> {actionLabel.raise}</> : <span>Aumentar</span>}
    </Button>
  </>;
}

export function ActionBar({
                            onAct,
                            available,
                            callAmount,
                            minRaise,
                            maxRaise,
                            raiseStep,
                            actionKey,
                            isTurn,
                            connected,
                            pending,
                            error,
                            onDismissError
                          }: Props) {
  const unavailable = !connected || !isTurn || pending !== null;
  const context = !connected ? 'Reconectando antes de liberar as ações…' : pending ? actionLabel[pending] : !isTurn ? 'Aguarde sua vez.' : 'Sua vez de agir.';
  const label = (action: PokerAction, idle: string) => pending === action ? <><LoaderCircle
    className="action-spinner"/> {actionLabel[action]}</> : <span>{idle}</span>;
  
  return <div className="action-bar" role="group" aria-label="Ações da rodada" aria-busy={pending !== null}>
    <p id="action-context" className="action-context" aria-live="polite">{context}</p>
    <div className="action-choices" role="group" aria-label="Ações rápidas">
      <Button type="button" variant="outline" disabled={unavailable || !available.fold}
              aria-describedby="action-context"
              onClick={() => onAct('fold')}>{label('fold', 'Desistir')}</Button>
      <Button type="button" variant="outline" disabled={unavailable || !available.check}
              aria-describedby="action-context"
              onClick={() => onAct('check')}>{label('check', 'Mesa')}</Button>
      <Button type="button" variant="outline" disabled={unavailable || !available.call}
              aria-describedby="action-context"
              onClick={() => onAct('call')}
              className="call">{label('call', callAmount > 0 ? `Pagar ${callAmount.toLocaleString('pt-BR')}` : 'Pagar')}</Button>
    </div>
    <RaiseControl key={actionKey} minRaise={minRaise} maxRaise={maxRaise} raiseStep={raiseStep}
                  disabled={unavailable || !available.raise} pending={pending === 'raise'}
                  onRaise={amount => onAct('raise', amount)}/>
    {error && <div className="action-error" role="alert">
        <CircleAlert aria-hidden="true"/><p>{error.message}</p>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar aviso"
                onClick={onDismissError}><X/></Button>
    </div>}
  </div>
}
