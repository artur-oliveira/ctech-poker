import {useState} from 'react';
import {
  ArrowRight,
  ArrowUp,
  Check,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  Eye,
  HelpCircle,
  LogIn,
  LogOut,
  LucideIcon,
  MessageSquare,
  NotebookPen,
  Pause,
  Play,
  Repeat2,
  Smile,
  TrendingDown,
  TrendingUp,
  Trophy,
  UserPen,
  WifiOff,
  X,
} from 'lucide-react';

import {Action, HandHistoryAction} from '@/lib/api/table';
import type {HandMetaStreet} from '@/lib/api/handMeta';
import {isTableReaction, TABLE_REACTIONS} from '@/lib/reactions';

const ACTION_META: Record<Action, { label: string; Icon: LucideIcon }> = {
  // Sistema
  join: {
    label: 'Entrou na mesa',
    Icon: LogIn,
  },
  leave: {
    label: 'Saiu da mesa',
    Icon: LogOut,
  },
  ready: {
    label: 'Pronto para jogar',
    Icon: Play,
  },
  not_ready: {
    label: 'Não está pronto',
    Icon: Pause,
  },
  sit_out: {
    label: 'Fora da rodada',
    Icon: Pause,
  },
  disconnect_sit_out: {
    label: 'Desconectou (Sit Out)',
    Icon: WifiOff,
  },
  keep_seat: {
    label: 'Confirmou presença',
    Icon: Clock3,
  },
  set_run_it_twice: {
    label: 'Ajustou Rodar Duas Vezes',
    Icon: Repeat2,
  },
  
  // Fluxo da partida
  next_hand: {
    label: 'Nova mão',
    Icon: ChevronRight,
  },
  runout_step: {
    label: 'Cartas restantes do board reveladas',
    Icon: ArrowRight,
  },
  request_exit: {
    label: 'Pediu para sair da mesa',
    Icon: LogOut,
  },
  escalate_blinds: {
    label: 'Blinds aumentaram',
    Icon: TrendingUp,
  },
  post_big_blind: {
    label: 'Postou o Big Blind',
    Icon: CircleDollarSign,
  },
  
  // Ações do jogador
  check: {
    label: 'Passou (check)',
    Icon: Check,
  },
  fold: {
    label: 'Desistiu (fold)',
    Icon: X,
  },
  call: {
    label: 'Pagou (call)',
    Icon: ArrowRight,
  },
  bet: {
    label: 'Apostou (bet)',
    Icon: CircleDollarSign,
  },
  raise: {
    label: 'Aumentou (raise)',
    Icon: ArrowUp,
  },
  all_in: {
    label: 'All-in',
    Icon: ArrowUp,
  },
  show_cards: {
    label: 'Mostrou as cartas',
    Icon: Eye,
  },
  peek_cards: {
    label: 'Espiou as cartas',
    Icon: Eye,
  },
  
  // Mesa social
  chat: {
    label: 'Falou no chat',
    Icon: MessageSquare,
  },
  reaction: {
    label: 'Reagiu',
    Icon: Smile,
  },
  set_identity: {
    label: 'Atualizou o perfil',
    Icon: UserPen,
  },
  
  // Resultado da mão
  won: {
    label: 'Venceu',
    Icon: Trophy,
  },
  tie: {
    label: 'Empatou',
    Icon: Trophy,
  },
  lost: {
    label: 'Perdeu a mão',
    Icon: TrendingDown,
  },
};

function formatTime(unixMillis: number) {
  return new Date(unixMillis).toLocaleTimeString('pt-BR', {hour: '2-digit', minute: '2-digit', second: '2-digit'});
}

const SYSTEM_ACTIONS = new Set<Action>([
  'join', 'leave', 'ready', 'not_ready', 'sit_out', 'disconnect_sit_out', 'keep_seat',
  'set_run_it_twice', 'next_hand', 'escalate_blinds', 'set_identity', 'request_exit'
]);
const SOCIAL_ACTIONS = new Set<Action>(['chat', 'reaction']);

const STREET_LABELS: Record<string, string> = {
  pre_flop: 'Pré-flop',
  preflop: 'Pré-flop',
  flop: 'Flop',
  turn: 'Turn',
  river: 'River',
  showdown: 'Showdown',
  complete: 'Showdown'
};

// ActionTimeline groups actions under its own internal keys (`pre_flop`, not
// `preflop`); this maps them onto the canonical handmeta.HAND_META_STREETS
// vocabulary #349's per-street note is keyed by.
const STREET_TO_META: Record<string, HandMetaStreet> = {
  pre_flop: 'preflop', flop: 'flop', turn: 'turn', river: 'river', showdown: 'showdown'
};

// Collapsed by default unless a note already exists (#349's mobile
// requirement: the note field must not push the action list out of the
// viewport). Empty text on blur means no annotation, never a placeholder —
// mirrors the server's own delete-on-empty convention.
function StreetNoteEditor({street, streetLabel, value, onSaveAction, saving, error}: {
  street: HandMetaStreet;
  streetLabel: string;
  value: string;
  onSaveAction: (street: HandMetaStreet, text: string) => void;
  saving: boolean;
  error: string | null;
}) {
  const [draft, setDraft] = useState(value);
  const [open, setOpen] = useState(Boolean(value));
  // Derived during render (React's documented "adjust state when a prop
  // changes" pattern) rather than a useEffect, which would fire one render
  // late and risk clobbering an in-flight draft with a stale re-render.
  const [syncedValue, setSyncedValue] = useState(value);
  if (value !== syncedValue) {
    setSyncedValue(value);
    setDraft(value);
  }
  const id = `street-note-${street}`;
  return <details className="action-street-note" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
    <summary><NotebookPen aria-hidden="true"/> {value ? 'Sua nota' : 'Adicionar nota'}</summary>
    <div className="action-street-note-body">
      <label htmlFor={id}>Nota sobre {streetLabel}</label>
      <textarea
        id={id}
        value={draft}
        maxLength={300}
        placeholder="Ex.: 3-bet de blefe, deveria ter pago"
        onChange={e => setDraft(e.target.value)}
        onBlur={() => {
          if (draft.trim() !== (value || '')) onSaveAction(street, draft);
        }}
      />
      {saving && <span className="action-street-note-status" role="status">Salvando…</span>}
      {/* Same "erro parcial" visual pattern hand-history-partial-error uses
          for the action list itself (#349's a11y criterion). */}
      {error && <div className="hand-history-partial-error" role="alert">
        <p className="form-error">{error}</p>
      </div>}
    </div>
  </details>;
}

function actionStreet(action: HandHistoryAction, previous: string): string {
  if (action.action === 'won' || action.action === 'lost' || action.action === 'tie'
    || action.action === 'show_cards') return 'showdown';
  const frameStage = action.frame?.stage;
  if (frameStage && STREET_LABELS[frameStage]) {
    if (frameStage === 'complete') return 'showdown';
    return frameStage === 'preflop' ? 'pre_flop' : frameStage;
  }
  return previous;
}

function ActionRows({actions, resolveName}: {
  actions: HandHistoryAction[];
  resolveName: (playerId: string) => string;
}) {
  return <ol className="action-timeline">
    {actions.map(a => {
      const meta = ACTION_META[a.action] || {label: a.action.replaceAll('_', ' '), Icon: HelpCircle};
      const Icon = meta.Icon;
      const reaction = a.reaction_id && isTableReaction(a.reaction_id) ? TABLE_REACTIONS[a.reaction_id] : null;
      return <li key={a.seq} className={`action-row action-${a.action}`}>
        <span className="action-row-icon" aria-hidden="true"><Icon/></span>
        <span className="action-row-who">{resolveName(a.player_id)}</span>
        <span className="action-row-what">
          {meta.label}
          {reaction && <>
            {' '}<span className="action-row-emoji" role="img" aria-label={reaction.label}>{reaction.glyph}</span>
            {a.target_player_id && <> para {resolveName(a.target_player_id)}</>}
          </>}
          {a.amount > 0 && <b>{a.amount.toLocaleString('pt-BR')}</b>}
        </span>
        <span className="action-row-when">{a.timestamp ? formatTime(a.timestamp) : '—'}</span>
      </li>;
    })}
  </ol>;
}

export function ActionTimeline({actions, resolveName, streetNotes, onSaveStreetNoteAction, savingStreet, noteError}: {
  actions: HandHistoryAction[];
  resolveName: (playerId: string) => string;
  /** #349: one short note per street, keyed by the canonical handmeta streets
   * — not renderable at all unless the caller wires a save handler. */
  streetNotes?: Partial<Record<HandMetaStreet, string>>;
  onSaveStreetNoteAction?: (street: HandMetaStreet, text: string) => void;
  savingStreet?: HandMetaStreet | null;
  noteError?: { street: HandMetaStreet; message: string } | null;
}) {
  if (!actions.length) return <p className="action-timeline-empty">Nenhuma ação registrada para esta mão.</p>;

  const system = actions.filter(action => SYSTEM_ACTIONS.has(action.action));
  const social = actions.filter(action => SOCIAL_ACTIONS.has(action.action));
  const play = actions.filter(action => !SYSTEM_ACTIONS.has(action.action) && !SOCIAL_ACTIONS.has(action.action));
  const streets = new Map<string, HandHistoryAction[]>();
  let currentStreet = 'pre_flop';
  for (const action of play) {
    currentStreet = actionStreet(action, currentStreet);
    const streetActions = streets.get(currentStreet) || [];
    streetActions.push(action);
    streets.set(currentStreet, streetActions);
  }

  return <div className="action-timeline-groups">
    {[...streets.entries()].map(([street, streetActions]) => {
      const label = STREET_LABELS[street] || street.replaceAll('_', ' ');
      const metaStreet = STREET_TO_META[street];
      return <section key={street} className="action-street">
        <h3>{label}</h3>
        <ActionRows actions={streetActions} resolveName={resolveName}/>
        {onSaveStreetNoteAction && metaStreet && <StreetNoteEditor
          street={metaStreet}
          streetLabel={label}
          value={streetNotes?.[metaStreet] || ''}
          onSaveAction={onSaveStreetNoteAction}
          saving={savingStreet === metaStreet}
          error={noteError?.street === metaStreet ? noteError.message : null}
        />}
      </section>;
    })}
    {system.length > 0 && <details className="action-secondary">
      <summary>Eventos do sistema <span>{system.length}</span></summary>
      <ActionRows actions={system} resolveName={resolveName}/>
    </details>}
    {social.length > 0 && <details className="action-secondary">
      <summary>Chat e reações <span>{social.length}</span></summary>
      <ActionRows actions={social} resolveName={resolveName}/>
    </details>}
  </div>;
}
