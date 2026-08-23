'use client';
import {useEffect, useMemo, useState, type CSSProperties} from 'react';
import {ChevronLeft, ChevronRight, Coins, Pause, Play, RotateCcw, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {TableStage} from '@/components/table/TableStage';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {RevealWinnerButton} from '@/components/hands/RevealWinnerButton';
import type {Action, HandHistoryAction, TableSnapshot} from '@/lib/api/table';
import type {HandItem} from '@/lib/api/player';
import {isTableReaction, TABLE_REACTIONS} from '@/lib/reactions';
import {playerName} from '@/lib/utils';

const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'Início', pre_flop: 'Pré-flop', flop: 'Flop',
  turn: 'Turn', river: 'River', showdown: 'Showdown', complete: 'Resultado'
};

// Every action the server can log, phrased as the predicate of "<player> …".
// Missing entries used to fall through to the raw snake_case key, which is how
// join/leave/next_hand/blind actions reached the screen untranslated.
const ACTION_LABELS: Record<Action, string> = {
  check: 'deu check', fold: 'foldou', call: 'pagou', bet: 'apostou',
  raise: 'aumentou para', all_in: 'foi all-in com', won: 'venceu',
  tie: 'empatou', show_cards: 'mostrou as cartas', runout_step: 'abriu o board',
  set_run_it_twice: 'ajustou a preferência de rodar duas vezes',
  join: 'entrou na mesa', leave: 'saiu da mesa',
  ready: 'ficou pronto para jogar', not_ready: 'não está pronto',
  sit_out: 'ficou fora da rodada', disconnect_sit_out: 'caiu a conexão e ficou fora',
  keep_seat: 'confirmou presença na mesa', next_hand: 'começou a próxima mão',
  escalate_blinds: 'viu os blinds aumentarem', post_big_blind: 'postou o big blind',
  chat: 'falou no chat', reaction: 'reagiu', set_identity: 'atualizou o perfil'
};

// Matches .board-card-reveal's animation in globals.css: each new card
// starts BOARD_CARD_STAGGER_MS after the last and takes BOARD_CARD_REVEAL_MS
// to finish, so the flop (3 cards) needs noticeably longer than the turn or
// river (1 card) to fully deal before the replay advances.
const BOARD_CARD_REVEAL_MS = 780;
const BOARD_CARD_STAGGER_MS = 320;
const REVEAL_BUFFER_MS = 200;
const ACTION_STEP_MS = 900;

function stepDelayMs(currentBoardLen?: number, nextBoardLen?: number) {
  const cardsAdded = Math.max(0, (nextBoardLen ?? 0) - (currentBoardLen ?? 0));
  if (cardsAdded === 0) return ACTION_STEP_MS;
  return BOARD_CARD_STAGGER_MS * (cardsAdded - 1) + BOARD_CARD_REVEAL_MS + REVEAL_BUFFER_MS;
}

export function HandReplayer({
                               hand,
                               actions,
                               viewerId
                             }: {
  hand: HandItem;
  actions: HandHistoryAction[];
  viewerId?: string;
}) {
  const replayActions = useMemo(() => {
    const filteredActions: HandHistoryAction[] = [];
    for (const act of actions.filter(action => action.frame)) {
      filteredActions.push(act);
      if (act.frame?.stage === 'complete') {
        break;
      }
    }
    return filteredActions;
  }, [actions]);
  // Reactions are cosmetic, so the server never attaches a replay frame to them
  // and the stepper above skips them entirely. Bucket each one under the frame
  // it followed, and the emojis reappear at the beat where they were thrown.
  const reactionsByStep = useMemo(() => {
    const byStep = new Map<number, HandHistoryAction[]>();
    let step = actions.find(action => action.frame)?.seq;
    if (step === undefined) return byStep;
    for (const action of actions) {
      if (action.frame) {
        step = action.seq;
        continue;
      }
      if (action.action !== 'reaction' || !action.reaction_id || !isTableReaction(action.reaction_id)) continue;
      byStep.set(step, [...(byStep.get(step) || []), action]);
    }
    return byStep;
  }, [actions]);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [revealedWinnerCards, setRevealedWinnerCards] = useState<[string, string] | null>(null);
  const lastIndex = Math.max(0, replayActions.length - 1);
  const safeIndex = Math.min(index, lastIndex);
  
  useEffect(() => {
    if (!playing || replayActions.length < 2) return undefined;
    const timer = window.setTimeout(() => {
      setIndex(current => {
        if (current >= lastIndex) {
          setPlaying(false);
          return current;
        }
        return current + 1;
      });
    }, stepDelayMs(replayActions[safeIndex]?.frame?.board?.length, replayActions[safeIndex + 1]?.frame?.board?.length) / speed);
    return () => window.clearTimeout(timer);
  }, [playing, safeIndex, lastIndex, replayActions, speed]);
  
  if (!replayActions.length) return <section className="hand-replayer unavailable">
    <div>
      <h2>Replay da mão</h2>
      <p>Esta mão foi registrada antes dos frames de replay serem habilitados. As próximas mãos poderão ser reproduzidas
        ação por ação.</p>
    </div>
  </section>;
  
  const current = replayActions[safeIndex];
  const frame = current.frame!;
  const opponents = new Map((hand.opponents || []).map(opponent => [opponent.player_id, opponent]));
  const shownPlayers = new Set(replayActions.slice(0, safeIndex + 1)
    .filter(action => action.action === 'show_cards').map(action => action.player_id));
  const showFinalCards = frame.stage === 'complete' || frame.stage === 'showdown';
  const actionLabel = ACTION_LABELS[current.action] || current.action.replaceAll('_', ' ');
  const stepReactions = reactionsByStep.get(current.seq) || [];
  const actor = playerName(current.player_id, viewerId, opponents.get(current.player_id)?.name ||
    frame.seats?.find(seat => seat.player_id === current.player_id)?.name);
  const winnerOpponent = frame.stage === 'complete'
    ? [...opponents.values()].find(o => o.won && (!o.hole_cards || o.hole_cards.length === 0))
    : undefined;

  const holeCardsFor = (playerId: string) => {
    if (playerId === viewerId) return hand.hole_cards;
    if (winnerOpponent && playerId === winnerOpponent.player_id && revealedWinnerCards) return revealedWinnerCards;
    if (!showFinalCards && !shownPlayers.has(playerId)) return undefined;
    return opponents.get(playerId)?.hole_cards;
  };
  const replaySnapshot: TableSnapshot = {
    stage: frame.stage,
    board: frame.board || [],
    board_two: frame.board_two,
    board_split_at: frame.board_split_at,
    current_player_id: frame.current_player_id,
    dealer_player_id: frame.dealer_player_id,
    small_blind_player_id: frame.small_blind_player_id,
    big_blind_player_id: frame.big_blind_player_id,
    payouts: frame.payouts,
    winners: frame.winners,
    pots: frame.pot > 0 ? [{
      amount: frame.pot,
      eligible_player_ids: (frame.seats || []).filter(seat => seat.dealt_in).map(seat => seat.player_id)
    }] : [],
    seats: (frame.seats || []).map(seat => ({
      ...seat,
      hole_cards: holeCardsFor(seat.player_id),
      hole_cards_revealed: holeCardsFor(seat.player_id)?.map(card => card.toLowerCase() !== 'back')
    }))
  };
  
  const replayPosition = (safeIndex + 1) / replayActions.length;

  return <section className={`hand-replayer${frame.stage === 'complete' ? ' is-complete' : ''}`}
                  aria-label="Replay interativo da mão">
    <header>
      <div className="replay-heading">
        <span className="replay-mark" aria-hidden="true"><Sparkles/></span>
        <div>
          <h2>Replay da mão</h2>
          <p><span className="replay-stage">{STAGE_LABELS[frame.stage] || frame.stage}</span>
            <span>Ação {safeIndex + 1} de {replayActions.length}</span></p>
        </div>
      </div>
      <div className="replay-header-end">
        {frame.stage === 'complete' && <OutcomeBadge outcome={hand.outcome}/>}
        {winnerOpponent && <RevealWinnerButton
          handId={hand.hand_id}
          winnerName={winnerOpponent.name}
          alreadyRevealed={Boolean(revealedWinnerCards)}
          onRevealedAction={cards => setRevealedWinnerCards(cards)}
        />}
        <span className="replay-pot"><Coins aria-hidden="true"/> <span>Pote</span> <b
          key={`${current.seq}-${frame.pot}`}>{frame.pot.toLocaleString('pt-BR')}</b></span>
      </div>
    </header>
    <div className="replay-live-table">
      <TableStage snapshot={replaySnapshot} viewer={viewerId} pot={frame.pot} bigBlind={25}
                  nowMs={current.timestamp} outcome={null} holdOutcomeOpen={false}/>
      <p key={`action-${current.seq}`} className="replay-action" aria-live="polite">
        <b>{actor}</b> {actionLabel}
        {current.amount > 0 && <> <strong>{current.amount.toLocaleString('pt-BR')}</strong></>}
      </p>
      {stepReactions.length > 0 && <ul key={`reactions-${current.seq}`} className="replay-reactions">
        {stepReactions.map(reaction => {
          const meta = TABLE_REACTIONS[reaction.reaction_id as keyof typeof TABLE_REACTIONS];
          const from = playerName(reaction.player_id, viewerId, opponents.get(reaction.player_id)?.name ||
            frame.seats?.find(seat => seat.player_id === reaction.player_id)?.name);
          return <li key={reaction.seq}>
            <span aria-hidden="true">{meta.glyph}</span>
            <small>{from} · {meta.label}</small>
          </li>;
        })}
      </ul>}
    </div>
    <div className="replay-controls">
      <div className="replay-transport" role="group" aria-label="Controles de reprodução">
        <Button type="button" variant="ghost" size="icon" aria-label="Voltar ao início"
                onClick={() => {
                  setPlaying(false);
                  setIndex(0);
                }}><RotateCcw/></Button>
        <Button type="button" variant="ghost" size="icon" aria-label="Ação anterior"
                disabled={safeIndex === 0} onClick={() => {
          setPlaying(false);
          setIndex(value => Math.max(0, value - 1));
        }}>
          <ChevronLeft/>
        </Button>
        <Button type="button" className="replay-play"
                aria-label={playing ? 'Pausar replay' : 'Reproduzir replay'}
                onClick={() => {
                  if (safeIndex === lastIndex) setIndex(0);
                  setPlaying(value => !value);
                }}>{playing ? <Pause/> : <Play/>}<span>{playing ? 'Pausar' : 'Reproduzir'}</span></Button>
        <Button type="button" variant="ghost" size="icon" aria-label="Próxima ação"
                disabled={safeIndex === lastIndex}
                onClick={() => {
                  setPlaying(false);
                  setIndex(value => Math.min(lastIndex, value + 1));
                }}>
          <ChevronRight/>
        </Button>
      </div>
      <label className="replay-scrubber">
        <span><b>{STAGE_LABELS[frame.stage] || frame.stage}</b> · ação {safeIndex + 1} de {replayActions.length}</span>
        <input type="range" min={0} max={lastIndex} value={safeIndex}
               aria-valuetext={`Ação ${safeIndex + 1} de ${replayActions.length}`}
               style={{'--replay-position': `${replayPosition * 100}%`} as CSSProperties}
               onChange={event => {
                 setPlaying(false);
                 setIndex(Number(event.target.value));
               }}/>
      </label>
      <button type="button" className="replay-speed" onClick={() => setSpeed(value => value === 1 ? 2 : 1)}
              aria-label={`Velocidade ${speed} vezes`}>
        <span>Velocidade</span><b>{speed}×</b>
      </button>
    </div>
    <span className="replay-progress" aria-hidden="true"
          style={{transform: `scaleX(${replayPosition})`}}/>
  </section>;
}
