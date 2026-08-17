'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {CircleAlert, Clock3, LoaderCircle, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {PokerAction} from '@/lib/api/table';
import type {ActionError} from '@/lib/hooks/useTableRealtime';
import {betShortcutAmount} from '@/lib/betShortcuts';
import {VoiceActionButton} from '@/components/table/VoiceActionButton';
import {type ActionPreselection, resolvePreselection} from '@/lib/actionPreselection';
import {useLiveNow} from '@/lib/hooks/useLiveNow';

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
}

const actionLabel: Record<PokerAction, string> = {
  fold: 'Desistindo…', check: 'Confirmando…', call: 'Pagando…', raise: 'Aumentando…'
};

// "Fold"/"Check" stay in English deliberately: BR players call them that at
// the table even in Portuguese play, while "Pagar"/"Aumentar" carry an
// amount, where a loanword reads worse. Confirmed intentional; don't
// "fix" this into Desistir/Passar without checking first.

/** True when the key press belongs to a text field (chat input, etc.), never the raise slider. */
function isTypingTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && !!target.closest('input:not([type=range]), textarea, select, [contenteditable]');
}

function isPlainKey(event: KeyboardEvent) {
  return !event.metaKey && !event.ctrlKey && !event.altKey && !event.repeat && !isTypingTarget(event.target);
}

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
                                prospectiveCallAmount,
                                selection,
                                selectionAmount,
                                onSelectAction,
                                onAct
                              }: {
  canPreselect: boolean;
  supportsCallPreselection: boolean;
  isTurn: boolean;
  connected: boolean;
  pending: PokerAction | null;
  available: ActionAvailability;
  callAmount: number;
  prospectiveCallAmount: number;
  selection: ActionPreselection | null;
  selectionAmount: number;
  onSelectAction: (selection: ActionPreselection | null, amount?: number) => boolean;
  onAct: (action: PokerAction) => boolean;
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
      onAct(action);
    }
  }, [selection, selectionAmount, isTurn, connected, pending, available, callAmount, onAct]);
  
  if (!canPreselect && !selection) return null;
  const option = (value: ActionPreselection, label: string, description: string, amount = 0) =>
    <button type="button" className={selection === value ? 'selected' : ''}
            aria-pressed={selection === value} title={description}
            disabled={!connected || pending !== null}
            onClick={() => onSelectAction(selection === value ? null : value, selection === value ? 0 : amount)}>
      <span>{label}<small>{description}</small></span>
    </button>;
  return <div className="action-preselectors" role="group" aria-label="Preparar próxima ação">
    <span>Próxima ação</span>
    {option('check_fold', 'Check / Fold', 'Check se for grátis; caso contrário, fold')}
    {option('fold', 'Fold', 'Desistir quando chegar sua vez')}
    {supportsCallPreselection && prospectiveCallAmount > 0 && option('call', `Call ${prospectiveCallAmount.toLocaleString('pt-BR')}`,
      'Pagar somente este valor; cancela se a aposta aumentar', prospectiveCallAmount)}
    {supportsCallPreselection && option('call_any', 'Call Any', 'Pagar qualquer valor quando chegar sua vez')}
  </div>;
}

/** Raise control. Keyed by `actionKey` in the parent so the chosen amount
 * resets to the street minimum on every new decision without an effect. */
function RaiseControl({minRaise, maxRaise, raiseStep, presets, disabled, pending, onRaise}: {
  minRaise: number; maxRaise: number; raiseStep: number; disabled: boolean; pending: boolean;
  presets: { label: string; value: number }[];
  onRaise: (amount: number) => void;
}) {
  const [amount, setAmount] = useState(minRaise);
  const [expanded, setExpanded] = useState(false);
  const safeAmount = Math.min(maxRaise, Math.max(minRaise, amount));
  const inactive = disabled || maxRaise < minRaise;
  // Raising to the max is shoving the whole stack, so call it what it is
  // instead of a "Pay" label with a number that happens to equal the stack.
  const isAllIn = safeAmount >= maxRaise;
  // A short stack clamps several fractions (½ pote, ⅔ pote, Pote...) down to
  // the same all-in total. Keeping the LAST match instead of the first means
  // the surviving button is the highest-named preset for that value (ideally
  // Máx), not e.g. "Mín" silently standing in for an all-in it doesn't read
  // as — the raise button already says "All In" once clicked, but the preset
  // itself shouldn't lie about which one it is.
  const uniquePresets = presets
    .map(preset => ({...preset, raw: preset.value, value: Math.min(maxRaise, Math.max(minRaise, preset.value))}))
    .filter((preset, index, all) => all.findLastIndex(item => item.value === preset.value) === index);
  // Presets carry their pre-clamp `raw` value into `amount` (not the already-
  // clamped `value` used for button dedup/display) so a short stack clamp
  // shows up as amount !== safeAmount below, instead of vanishing silently.
  const wasClamped = amount !== safeAmount;
  
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
          setAmount(betShortcutAmount(key, safeAmount, minRaise, maxRaise, raiseStep, false,
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
        setAmount(value => betShortcutAmount(event.key, value, minRaise, maxRaise, raiseStep,
          event.ctrlKey) ?? value);
      }
    }
    
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [inactive, safeAmount, onRaise, minRaise, maxRaise, raiseStep, presets]);
  
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
        {uniquePresets.map(preset => <button key={preset.label} type="button" disabled={inactive}
                                             onClick={() => setAmount(preset.raw)}>{preset.label}</button>)}
      </div>
      <Input id="raise-amount" aria-describedby="action-context" type="range"
             aria-keyshortcuts="a h ArrowUp ArrowDown ArrowLeft ArrowRight"
             min={minRaise} max={maxRaise} step={raiseStep} value={safeAmount}
             disabled={inactive}
             onChange={event => setAmount(Number(event.target.value))}
             aria-valuetext={`Total ${safeAmount.toLocaleString('pt-BR')} fichas${isAllIn ? ', All In' : ''}`}/>
      <output id="raise-amount-output" htmlFor="raise-amount">
        <small>{isAllIn ? 'All In' : 'Total'}</small>
        {safeAmount.toLocaleString('pt-BR')}
        {wasClamped && <small className="bet-clamped-note" role="status">
          {safeAmount >= maxRaise ? 'ajustado ao máximo' : 'ajustado ao mínimo'}
        </small>}
      </output>
    </label>
    <Button type="button" disabled={inactive} aria-keyshortcuts="r"
            aria-describedby="action-context" onClick={handleRaiseClick}
            className={`raise${expanded ? '' : ' raise-collapsed'}`}>
      {pending ? <><LoaderCircle className="action-spinner"/> {isAllIn ? 'Indo All In…' : actionLabel.raise}</> :
        // "para" makes explicit this is a raise-to-total, not an amount added on top
        // of the current bet (unlike Pagar's amount above, which is additive). Same
        // Verb + Amount shape as Pagar otherwise read as the same kind of number.
        <span>{expanded ? (isAllIn ? `All In ${safeAmount.toLocaleString('pt-BR')}` : `Aumentar para ${safeAmount.toLocaleString('pt-BR')}`) : (isAllIn ? 'All In' : 'Aumentar')}
          <kbd aria-hidden="true">R</kbd></span>}
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
                            voiceCommands
                          }: Props) {
  const legalActions = (Object.keys(available) as PokerAction[]).filter(action => available[action]);
  const preparedAction = selectionScope && preselection && isTurn ?
    resolvePreselection(preselection, legalActions, callAmount, preselectionAmount) : null;
  // Suppress the ordinary controls in the very first paint of the viewer's
  // turn, before the effect above submits the prepared action. If submission
  // is rejected, the visible error releases the controls for a manual choice.
  const executingPreparedAction = preparedAction !== null && error === null;
  const unavailable = !connected || !isTurn || pending !== null || executingPreparedAction;
  const context = !connected ? 'Reconectando antes de liberar as ações…' : pending ? actionLabel[pending] :
    executingPreparedAction ? 'Executando sua ação preparada…' : !isTurn ?
      'Aguarde sua vez.' : effectiveStack > 0 ?
        `Sua vez de agir. Stack efetivo: ${effectiveStack.toLocaleString('pt-BR')} fichas.` : 'Sua vez de agir.';
  const label = (action: PokerAction, idle: string, key?: string) => {
    if (pending === action) {
      return <><LoaderCircle className="action-spinner"/> {actionLabel[action]}</>;
    }
    return <span>{idle}{key && <kbd aria-hidden="true">{key}</kbd>}</span>;
  };
  const onRaise = useCallback((amount: number) => onActAction('raise', amount), [onActAction]);
  const canFold = available.fold, canCheck = available.check, canCall = available.call;
  // Nothing to do this street at all (waiting for players, folded, showdown/
  // complete), so collapse the choice row + raise slider instead of painting
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
  
  return <div className={`action-bar${isTurn ? ' is-turn' : ''}`} role="group"
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
                          callAmount={callAmount} prospectiveCallAmount={prospectiveCallAmount}
                          selection={preselection} selectionAmount={preselectionAmount}
                          onSelectAction={onPreselectAction} onAct={onActAction}/>
    {!noLegalActions && !executingPreparedAction &&
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
        </div>}
    {!noLegalActions && !executingPreparedAction &&
        <RaiseControl key={actionKey} minRaise={minRaise} maxRaise={maxRaise} raiseStep={raiseStep}
                      disabled={unavailable || !available.raise} presets={raisePresets}
                      pending={pending === 'raise'} onRaise={onRaise}/>}
    {error && <div className="action-error" role="alert">
        <CircleAlert aria-hidden="true"/><p>{error.message}</p>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar aviso"
                onClick={onDismissErrorAction}><X/></Button>
    </div>}
  </div>;
}
