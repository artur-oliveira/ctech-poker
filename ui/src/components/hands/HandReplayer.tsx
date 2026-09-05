'use client';
import {useEffect, useId, useMemo, useState, type CSSProperties} from 'react';
import {
  ChevronLeft, ChevronRight, Coins, GraduationCap, Pause, Play, RotateCcw, Sparkles
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {TableStage} from '@/components/table/TableStage';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {RevealWinnerButton} from '@/components/hands/RevealWinnerButton';
import type {Action, HandHistoryAction, TableSnapshot} from '@/lib/api/table';
import type {HandItem} from '@/lib/api/player';
import {isTableReaction, TABLE_REACTIONS} from '@/lib/reactions';
import {deriveBigBlind} from '@/lib/replayBlinds';
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
  raise: 'aumentou para', all_in: 'foi all-in com', won: 'venceu', lost: 'perdeu a mão',
  tie: 'empatou', show_cards: 'mostrou as cartas', peek_cards: 'espiou as cartas',
  request_exit: 'pediu para sair da mesa',
  // Server-dealt beat, not a player action: the remaining board cards after an
  // all-in. Rendered without an actor (player_id is empty).
  runout_step: 'Cartas restantes do board reveladas',
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
// Under `prefers-reduced-motion` the card-reveal animations are suppressed by
// globals.css, so there is no deal to wait for — and the animation was also
// what made the cadence readable. A flat, unhurried step replaces both, so no
// frame advances before it has been read.
const REDUCED_MOTION_STEP_MS = 2_400;
const SPEEDS = [1, 2, 0.5] as const;

// v1 coaching mode (issue #351): a static, local question bank — no AI, no
// endpoint. One is picked per hero decision point by `seq`, so the same step
// always asks the same question.
const COACH_QUESTIONS = [
  'Pense nas pot odds: o valor que você pagaria compensa a chance de completar sua mão?',
  'Qual é a sua posição na mesa nesta rodada — isso muda a sua decisão aqui?',
  'Que range de mãos o vilão provavelmente tem, dado o padrão de apostas até agora?',
  'O que a aposta do adversário sugere sobre a força da mão dele?',
  'Você jogaria diferente se o seu stack fosse bem menor?',
  'Vale blefar aqui, ou é melhor jogar de forma direta com essa mão?'
] as const;
const HERO_DECISION_ACTIONS = new Set<Action>(['check', 'fold', 'call', 'bet', 'raise', 'all_in']);

function stepDelayMs(reduced: boolean, currentBoardLen?: number, nextBoardLen?: number) {
  if (reduced) return REDUCED_MOTION_STEP_MS;
  const cardsAdded = Math.max(0, (nextBoardLen ?? 0) - (currentBoardLen ?? 0));
  if (cardsAdded === 0) return ACTION_STEP_MS;
  return BOARD_CARD_STAGGER_MS * (cardsAdded - 1) + BOARD_CARD_REVEAL_MS + REVEAL_BUFFER_MS;
}

export function HandReplayer({
                               hand,
                               actions,
                               viewerId,
                               allowCoaching = false
                             }: {
  hand: HandItem;
  actions: HandHistoryAction[];
  viewerId?: string;
  // Opt-in gate for the coaching mode toggle (issue #351). Off by default so
  // `/share` (public, unauthenticated link) never renders it; the
  // authenticated replay page is the only caller that turns it on.
  allowCoaching?: boolean;
}) {
  const replayActions = useMemo(() => {
    const filteredActions: HandHistoryAction[] = [];
    let resolved = false;
    for (const act of actions.filter(action => action.frame)) {
      if (resolved) {
        // The hand is over. Keep the terminal beats that name who won or lost so
        // the replay ends on the outcome, not on the board runout; stop before
        // anything belonging to the next hand (join, next_hand, request_exit…).
        if (act.action === 'won' || act.action === 'lost' || act.action === 'tie'
          || act.action === 'show_cards') {
          filteredActions.push(act);
          continue;
        }
        break;
      }
      filteredActions.push(act);
      if (act.frame?.stage === 'complete') resolved = true;
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
  const bigBlind = useMemo(() => deriveBigBlind(actions, hand.big_blind), [actions, hand.big_blind]);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speedIndex, setSpeedIndex] = useState(0);
  const speed = SPEEDS[speedIndex];
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const [revealedWinnerCards, setRevealedWinnerCards] = useState<[string, string] | null>(null);
  const [coachingOn, setCoachingOn] = useState(false);
  const [resolvedCoachSteps, setResolvedCoachSteps] = useState<Set<number>>(() => new Set());
  const shortcutsId = useId();
  const coachPauseId = useId();
  const lastIndex = Math.max(0, replayActions.length - 1);
  const safeIndex = Math.min(index, lastIndex);
  const candidateStep = replayActions[safeIndex];
  const awaitingCoachAnswer = allowCoaching && coachingOn && candidateStep !== undefined
    && candidateStep.player_id === viewerId
    && HERO_DECISION_ACTIONS.has(candidateStep.action)
    && !resolvedCoachSteps.has(candidateStep.seq);

  // The coaching pause takes priority over playback: landing on a hero
  // decision point with coaching on visually pauses (derived below, no
  // effect needed) and blocks the autoplay timer (see its guard) until the
  // question is answered or skipped, without discarding the underlying
  // `playing` intent — resolving the step resumes playback if it was on.
  const effectivePlaying = playing && !awaitingCoachAnswer;

  function revealCoachStep(seq: number) {
    setResolvedCoachSteps(previous => new Set(previous).add(seq));
  }

  useEffect(() => {
    if (!playing || replayActions.length < 2 || awaitingCoachAnswer) return undefined;
    const timer = window.setTimeout(() => {
      setIndex(current => {
        if (current >= lastIndex) {
          setPlaying(false);
          return current;
        }
        return current + 1;
      });
    }, stepDelayMs(reduced, replayActions[safeIndex]?.frame?.board?.length, replayActions[safeIndex + 1]?.frame?.board?.length) / speed);
    return () => window.clearTimeout(timer);
  }, [playing, safeIndex, lastIndex, replayActions, speed, reduced, awaitingCoachAnswer]);

  // Space play/pause, ←/→ step, Home restart — the same transport the buttons
  // expose, for a keyboard already inside the replayer. A focused form control
  // (the scrubber, any text field) keeps its own keys, and a focused button
  // keeps Space as its activation; OS auto-repeat is dropped like
  // `useHoldRepeat` does, so a held arrow does not race through the hand.
  function onTransportKeyDown(event: React.KeyboardEvent<HTMLElement>) {
    if (event.repeat) return;
    const target = event.target as HTMLElement;
    const tag = target.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) return;
    if (event.key === ' ' && tag === 'BUTTON') return;
    if (event.key === ' ') {
      event.preventDefault();
      if (safeIndex === lastIndex) setIndex(0);
      setPlaying(value => !value);
      return;
    }
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      setPlaying(false);
      setIndex(value => Math.max(0, Math.min(value, lastIndex) - 1));
      return;
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      setPlaying(false);
      setIndex(value => Math.min(lastIndex, value + 1));
      return;
    }
    if (event.key === 'Home') {
      event.preventDefault();
      setPlaying(false);
      setIndex(0);
    }
  }


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
                  aria-label="Replay interativo da mão" aria-describedby={shortcutsId}
                  tabIndex={-1} onKeyDown={onTransportKeyDown}>
    <p id={shortcutsId} className="sr-only">
      Atalhos: barra de espaço reproduz ou pausa, seta esquerda e seta direita avançam uma ação, Home volta ao início.
    </p>
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
        {allowCoaching && <button type="button" className="replay-coach-toggle"
                                   aria-pressed={coachingOn}
                                   onClick={() => setCoachingOn(value => !value)}>
          <GraduationCap aria-hidden="true"/>
          <span>Modo Coaching {coachingOn ? 'ativado' : 'desativado'}</span>
        </button>}
        {frame.stage === 'complete' && <OutcomeBadge outcome={hand.outcome}/>}
        {winnerOpponent && <RevealWinnerButton
          handId={hand.hand_id}
          winnerName={winnerOpponent.name}
          alreadyRevealed={Boolean(revealedWinnerCards)}
          onRevealedAction={cards => setRevealedWinnerCards(cards)}
        />}
        <span className="replay-blind"><span>BB</span> <b>{bigBlind.toLocaleString('pt-BR')}</b></span>
        <span className="replay-pot"><Coins aria-hidden="true"/> <span>Pote</span> <b
          key={`${current.seq}-${frame.pot}`}>{frame.pot.toLocaleString('pt-BR')}</b></span>
      </div>
    </header>
    {awaitingCoachAnswer ? <div className="replay-coach-pause" aria-describedby={coachPauseId}>
      <p id={coachPauseId} className="sr-only" aria-live="assertive">
        Replay pausado num ponto de decisão. Responda a pergunta de coaching ou pule para ver a ação.
      </p>
      <GraduationCap aria-hidden="true"/>
      <p className="replay-coach-question">{COACH_QUESTIONS[current.seq % COACH_QUESTIONS.length]}</p>
      <div className="replay-coach-actions">
        <Button type="button" onClick={() => revealCoachStep(current.seq)}>Já pensei, revelar ação</Button>
        <Button type="button" variant="ghost" onClick={() => revealCoachStep(current.seq)}>Pular pergunta</Button>
      </div>
    </div> : <div className="replay-live-table">
      <TableStage snapshot={replaySnapshot} viewer={viewerId} pot={frame.pot} bigBlind={bigBlind}
                  nowMs={current.timestamp} outcome={null} holdOutcomeOpen={false}/>
      <p key={`action-${current.seq}`} className="replay-action" aria-live="polite">
        {current.player_id && <><b>{actor}</b> </>}{actionLabel}
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
    </div>}
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
        <Button type="button" className="replay-play" disabled={awaitingCoachAnswer}
                aria-label={effectivePlaying ? 'Pausar replay' : 'Reproduzir replay'}
                onClick={() => {
                  if (safeIndex === lastIndex) setIndex(0);
                  setPlaying(value => !value);
                }}>{effectivePlaying ? <Pause/> : <Play/>}<span>{effectivePlaying ? 'Pausar' : 'Reproduzir'}</span></Button>
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
      <button type="button" className="replay-speed"
              onClick={() => setSpeedIndex(value => (value + 1) % SPEEDS.length)}
              aria-label={`Velocidade ${speed.toLocaleString('pt-BR')} vezes, trocar`}>
        <span>Velocidade</span><b>{speed.toLocaleString('pt-BR')}×</b>
      </button>
    </div>
    <span className="replay-progress" aria-hidden="true"
          style={{transform: `scaleX(${replayPosition})`}}/>
  </section>;
}
