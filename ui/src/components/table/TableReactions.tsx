'use client';
import '@/app/table-reactions.css';
import {type CSSProperties, useEffect, useRef, useState} from 'react';
import {
  Crosshair, Eye, EyeOff, LockKeyhole, SmilePlus, Sparkles, Star, UserRound, X
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {EmojiGlyph} from '@/components/ui/EmojiGlyph';
import {ReactionFavoritesDialog} from '@/components/reactions/ReactionFavoritesDialog';
import type {SeatView} from '@/lib/api/table';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import {ownedReactionIDs} from '@/lib/api/reactionPurchases';
import {isHoverCapable} from '@/lib/utils';
import {useDismiss} from '@/lib/hooks/useDismiss';
import {
  PREMIUM_REACTION_IDS, TABLE_REACTIONS, type TableReactionEvent, type TableReactionID
} from '@/lib/reactions';

const REACTION_MUTE_KEY = 'poker:table-reactions-muted';
const CHIP_PIECES = Array.from({length: 12}, (_, index) => index);
type ReactionScope = 'self' | 'target';

const REACTION_ENTRIES = Object.entries(TABLE_REACTIONS) as [
  TableReactionID, typeof TABLE_REACTIONS[TableReactionID]
][];

const REACTION_THEATER = {
  clap: {accent: 'BRAVO', particles: ['✦', '·', '✦', '·', '✦', '·']},
  laugh: {accent: 'HA!', particles: ['HA', 'HA', 'HA', 'HA']},
  wow: {accent: '!', particles: ['✦', '!', '✦', '!']},
  angry: {accent: '', particles: ['', '', '', '']},
  cry: {accent: '', particles: ['', '', '', '', '']},
  nervous: {accent: '', particles: ['•', '•', '•', '•', '•']},
  cold: {accent: '', particles: ['❄', '❄', '❄', '❄', '❄', '❄']},
  fire: {accent: '', particles: ['', '', '', '', '', '']},
  respect: {accent: 'GG', particles: ['✦', '✦', '✦', '✦']},
  sleepy: {accent: '', particles: ['z', 'z', 'Z']},
  heartbeat: {accent: 'ALL IN', particles: ['♥', '·', '♥', '·']},
  shark: {accent: '', particles: ['≈', '≈', '≈', '≈']},
  pokerface: {accent: '', particles: ['♠', '♦', '♣', '♥']},
  chip: {accent: '', particles: []},
  coffee: {accent: '', particles: ['~', '~', '~']},
  clover: {accent: '', particles: ['🍀', '✦', '🍀', '✦', '🍀', '✦']},
  horseshoe: {accent: '', particles: ['★', '★', '★', '★', '★', '★']},
  tear: {accent: '', particles: []},
  tomato: {accent: 'SPLAT', particles: ['', '', '', '', '', '']},
  poop: {accent: 'ECA', particles: ['·', '·', '·', '·', '·']},
  rofl: {accent: 'HA!', particles: ['🤣', '🤣', '🤣']},
  duck: {accent: 'QUACK', particles: ['🪶', '🪶', '🪶', '🪶', '🪶']},
  turtle: {accent: '', particles: ['z', 'z', 'Z']},
  knife: {accent: '', particles: ['', '', '', '', '', '', '', '']},
  flowers: {accent: '', particles: ['🌸', '🌸', '🌸', '🌸', '🌸', '🌸']},
  spotlight: {accent: 'BOA', particles: ['✦', '✦', '✦', '✦']},
  crown: {accent: 'REI', particles: ['♠', '♦', '♣', '♥', '★']},
  bandage: {accent: '+1', particles: ['♥', '✦', '♥', '✦']},
  cucumber: {accent: 'SUSTO!', particles: ['', '', '', '', '', '']},
  boomerang: {accent: 'VOLTOU', particles: ['', '', '']}
} satisfies Record<TableReactionID, {accent: string; particles: string[]}>;

function ReactionChipStack({className = '', style}: {className?: string; style?: CSSProperties}) {
  return <span className={`reaction-table-chip-stack ${className}`} style={style} aria-hidden="true">
    {[0, 1, 2].map(index => <span key={index} className="chip"
                                  style={{'--i': index} as CSSProperties}/>)}</span>;
}

function ReactionGlyph({reactionId, glyph}: {reactionId: TableReactionID; glyph: string}) {
  return reactionId === 'chip'
    ? <ReactionChipStack/>
    : <EmojiGlyph glyph={glyph}/>;
}

function chipBurstStyle(index: number): CSSProperties {
  const angle = (index / CHIP_PIECES.length) * 360 + (index % 2 === 0 ? -9 : 9);
  const rad = (angle * Math.PI) / 180;
  const burstDist = 30 + (index % 4) * 9;
  const burstX = Math.cos(rad) * burstDist;
  const burstY = Math.sin(rad) * burstDist * 0.55 - 6;
  const fallExtra = 46 + (index % 3) * 20;
  return {
    '--piece': index,
    '--chip-burst-x': `${burstX.toFixed(1)}px`,
    '--chip-burst-y': `${burstY.toFixed(1)}px`,
    '--chip-fall-x': `${(burstX + ((index % 5) - 2) * 6).toFixed(1)}px`,
    '--chip-fall-y': `${(burstY + fallExtra).toFixed(1)}px`,
    '--chip-rot0': `${index % 2 === 0 ? -70 : 70}deg`,
    '--chip-rot1': `${(index % 2 === 0 ? -18 : 18) + index * 3}deg`,
  } as CSSProperties;
}

function ReactionImpact({reactionId}: {reactionId: TableReactionID}) {
  if (reactionId === 'chip') {
    return <span className="reaction-impact reaction-impact-chip" data-reaction-impact={reactionId} aria-hidden="true">
      <i className="reaction-chip-flash"/>
      {CHIP_PIECES.map(index => <ReactionChipStack key={index} className="reaction-jackpot-stack"
                                                   style={chipBurstStyle(index)}/>)}
    </span>;
  }

  const theater = REACTION_THEATER[reactionId];
  return <span className={`reaction-impact reaction-impact-${reactionId}`}
               data-reaction-impact={reactionId} aria-hidden="true">
    {theater.accent && <b className="reaction-impact-accent">{theater.accent}</b>}
    <span className="reaction-impact-particles">
      {theater.particles.map((particle, index) =>
        <i key={index} style={{'--piece': index} as CSSProperties}>{particle}</i>)}
    </span>
  </span>;
}

function ReactionEffect({item, seatEls}: {item: TableReactionEvent; seatEls: Map<string, HTMLElement>}) {
  const definition = TABLE_REACTIONS[item.reactionId];
  const positionEffect = (node: HTMLSpanElement | null) => {
    if (!node) return;
    const source = seatEls.get(item.playerId);
    const target = item.targetPlayerId ? seatEls.get(item.targetPlayerId) : undefined;
    if (!source || (definition.targeted && !target)) return;
    const from = source.getBoundingClientRect();
    const to = target?.getBoundingClientRect();
    const fromX = from.left + from.width / 2;
    const fromY = from.top + from.height / 2;
    const dx = to ? to.left + to.width / 2 - fromX : 0;
    const dy = to ? to.top + to.height / 2 - fromY : -72;
    const travel = Math.hypot(dx, dy);
    node.style.setProperty('--reaction-x', `${fromX}px`);
    node.style.setProperty('--reaction-y', `${fromY}px`);
    node.style.setProperty('--reaction-dx', `${dx}px`);
    node.style.setProperty('--reaction-dy', `${dy}px`);
    node.style.setProperty('--reaction-mid-x', `${(dx * .48).toFixed(1)}px`);
    node.style.setProperty('--reaction-mid-y', `${(dy * .48).toFixed(1)}px`);
    node.style.setProperty('--reaction-arc', `${Math.min(112, Math.max(58, travel * .22)).toFixed(1)}px`);
    node.style.visibility = 'visible';
  };

  return <span ref={positionEffect}
               className={`table-reaction-effect reaction-${item.reactionId} ${definition.targeted ? 'thrown' : 'emote'}`}
               data-reaction-id={item.reactionId} role="img" aria-label={definition.label}>
    <span className="reaction-projectile"><ReactionGlyph reactionId={item.reactionId} glyph={definition.glyph}/></span>
    <ReactionImpact reactionId={item.reactionId}/>
  </span>;
}

function ReactionChoice({id, state, disabled, onChoose}: {
  id: TableReactionID;
  state: 'free' | 'loading' | 'owned' | 'locked' | 'unavailable';
  disabled: boolean;
  onChoose: (id: TableReactionID) => void;
}) {
  const definition = TABLE_REACTIONS[id];
  return <button type="button" className={`reaction-choice reaction-choice-${id} ${state}`}
                 disabled={disabled} onClick={() => onChoose(id)}
                 aria-label={`${definition.label}. ${definition.caption}`}>
    <span className="reaction-choice-glyph">
      <ReactionGlyph reactionId={id} glyph={definition.glyph}/>
    </span>
    <span className="reaction-choice-copy">
      <strong>{definition.label}</strong>
      <small>{definition.caption}</small>
    </span>
    {state === 'loading' && <span className="reaction-choice-loading" aria-label="Carregando"/>}
    {state === 'locked' && <LockKeyhole className="reaction-choice-state" aria-label="Premium bloqueada"/>}
    {state === 'owned' && <Sparkles className="reaction-choice-state" aria-label="Premium liberada"/>}
  </button>;
}

export function TableReactions({items, seats, viewerId, connected, coolingDown, pendingReaction, onQuickSendAction,
                                 onPendingReactionChangeAction, open, onOpenChangeAction, premiumEnabled = false,
                                 premiumLoading = false, catalog = [], purchases = [], favorites = [],
                                 favoritesSaving = false, onLockedReactionAction, onFavoriteReactionsChangeAction}: {
  items: TableReactionEvent[];
  seats: SeatView[];
  viewerId?: string;
  connected: boolean;
  coolingDown: boolean;
  pendingReaction: TableReactionID | null;
  onQuickSendAction: (reaction: TableReactionID) => void;
  onPendingReactionChangeAction: (reaction: TableReactionID | null) => void;
  open: boolean;
  onOpenChangeAction: (open: boolean) => void;
  premiumEnabled?: boolean;
  premiumLoading?: boolean;
  catalog?: ReactionCatalogEntry[];
  purchases?: ReactionPurchase[];
  favorites?: TableReactionID[];
  favoritesSaving?: boolean;
  onLockedReactionAction?: (entry: ReactionCatalogEntry) => void;
  onFavoriteReactionsChangeAction?: (favorites: TableReactionID[]) => Promise<void> | void;
}) {
  const [muted, setMuted] = useState(() =>
    typeof window !== 'undefined' && window.localStorage.getItem(REACTION_MUTE_KEY) === 'true');
  const [scope, setScope] = useState<ReactionScope>('self');
  const hasOpponents = seats.some(seat => seat.player_id !== viewerId);
  const [favoritesOpen, setFavoritesOpen] = useState(false);
  const asideRef = useRef<HTMLElement>(null);
  useDismiss(asideRef, open, () => onOpenChangeAction(false));
  const [seatEls, setSeatEls] = useState<Map<string, HTMLElement>>(() => new Map());
  const seatIdsKey = seats.map(seat => seat.player_id).join(',');

  useEffect(() => {
    const map = new Map<string, HTMLElement>();
    document.querySelectorAll<HTMLElement>('.game-seat[data-player-id]').forEach(node => {
      if (node.dataset.playerId) map.set(node.dataset.playerId, node);
    });
    // Reads the seats DOM built by sibling components; there is no shared ref boundary.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSeatEls(map);
  }, [seatIdsKey]);

  const entries = new Map(catalog.map(entry => [entry.id, entry]));
  const owned = ownedReactionIDs(catalog);
  const visibleReactions = REACTION_ENTRIES.filter(([, definition]) =>
    scope === 'target' ? definition.targeted : !definition.targeted);
  const selfCount = REACTION_ENTRIES.filter(([, definition]) => !definition.targeted).length;
  const targetCount = REACTION_ENTRIES.length - selfCount;

  function toggleMute() {
    setMuted(value => {
      window.localStorage.setItem(REACTION_MUTE_KEY, String(!value));
      return !value;
    });
  }

  function premiumState(id: TableReactionID) {
    if (!premiumEnabled || !PREMIUM_REACTION_IDS.has(id)) return 'free' as const;
    if (premiumLoading) return 'loading' as const;
    if (owned.has(id)) return 'owned' as const;
    if (purchases.some(item => item.reaction_id === id && item.status === 'refunding')) return 'unavailable' as const;
    return entries.has(id) ? 'locked' as const : 'unavailable' as const;
  }

  function chooseReaction(id: TableReactionID) {
    const state = premiumState(id);
    if (state === 'loading' || state === 'unavailable') return;
    if (state === 'locked') {
      const entry = entries.get(id);
      if (entry) {
        onOpenChangeAction(false);
        onLockedReactionAction?.(entry);
      }
      return;
    }
    if (TABLE_REACTIONS[id].targeted) {
      onPendingReactionChangeAction(id);
      onOpenChangeAction(false);
    } else {
      onQuickSendAction(id);
      onOpenChangeAction(false);
    }
  }

  const pendingLabel = pendingReaction ? TABLE_REACTIONS[pendingReaction].label : '';

  return <>
    {!muted && <div className="table-reaction-layer" aria-live="off">
      {items.map(item => <ReactionEffect key={item.id} item={item} seatEls={seatEls}/>)}
    </div>}
    <aside ref={asideRef} className={`table-reactions${open ? ' open' : ''}${pendingReaction ? ' targeting' : ''}`}
           aria-label="Reações da mesa"
           onMouseEnter={() => isHoverCapable() && !pendingReaction && onOpenChangeAction(true)}
           onMouseLeave={() => isHoverCapable() && !pendingReaction && onOpenChangeAction(false)}>
      <Button type="button" variant="ghost" size="icon" className="reaction-toggle"
              aria-label={pendingReaction ? 'Cancelar reação direcionada' : open ? 'Fechar reações' : 'Abrir reações'}
              aria-expanded={open} aria-pressed={Boolean(pendingReaction)}
              onClick={() => pendingReaction ? onPendingReactionChangeAction(null) : onOpenChangeAction(!open)}>
        {open || pendingReaction ? <X/> : <SmilePlus/>}
      </Button>
      {pendingReaction && <span className="reaction-target-hint" role="status">
        <b>{pendingLabel}</b><small>Toque em um jogador</small>
      </span>}
      {open && <div className="reaction-panel">
        <header>
          <span><b>Teatro da mesa</b><small>Escolha o seu momento</small></span>
          <span>
            {onFavoriteReactionsChangeAction && <Button type="button" variant="ghost" size="icon"
                    aria-label="Editar reações favoritas" onClick={() => setFavoritesOpen(true)}>
              <Star aria-hidden="true"/>
            </Button>}
            <Button type="button" variant="ghost" size="icon"
                    aria-label={muted ? 'Mostrar efeitos de reações' : 'Ocultar efeitos de reações'}
                    aria-pressed={muted} onClick={toggleMute}>
              {muted ? <EyeOff/> : <Eye/>}
            </Button>
            <Button type="button" variant="ghost" size="icon" aria-label="Fechar painel de reações"
                    onClick={() => onOpenChangeAction(false)}><X/></Button>
          </span>
        </header>
        {favorites.length > 0 && <div className="reaction-favorites" role="group" aria-label="Reações favoritas">
          <span><Star aria-hidden="true"/> Seus atalhos</span>
          <div>{favorites.map(id => {
            const definition = TABLE_REACTIONS[id];
            if (!definition) return null;
            const state = premiumState(id);
            return <button type="button" key={id} title={definition.label}
                           className={`${state}${state === 'owned' ? ' premium-owned' : ''}`}
                           disabled={!connected || coolingDown || state === 'loading' || state === 'unavailable' ||
                             (definition.targeted && !hasOpponents)} onClick={() => chooseReaction(id)}>
              <ReactionGlyph reactionId={id} glyph={definition.glyph}/>
              <span>{definition.label}</span>
              {state === 'locked' && <LockKeyhole aria-label="Premium bloqueada"/>}
              {state === 'owned' && <Sparkles aria-label="Premium liberada"/>}
            </button>;
          })}</div>
        </div>}
        <div className="reaction-scope" role="tablist" aria-label="Tipo de reação">
          <button type="button" role="tab" aria-selected={scope === 'self'}
                  onClick={() => setScope('self')}>
            <UserRound aria-hidden="true"/><span>Na minha cadeira<small>{selfCount} tells</small></span>
          </button>
          <button type="button" role="tab" aria-selected={scope === 'target'}
                  onClick={() => setScope('target')}>
            <Crosshair aria-hidden="true"/><span>Mandar para alguém<small>{targetCount} gestos</small></span>
          </button>
        </div>
        <p className="reaction-scope-instruction">
          {scope === 'self'
            ? 'A reação nasce na sua cadeira e não interrompe a mão.'
            : hasOpponents
              ? 'Escolha um gesto e depois toque no jogador que vai recebê-lo.'
              : 'Outro jogador precisa estar sentado para receber um gesto.'}
        </p>
        <div className="reaction-catalog" role="tabpanel"
             aria-label={scope === 'self' ? 'Reações na minha cadeira' : 'Reações para outro jogador'}>
          {visibleReactions.map(([id, definition]) => {
            const state = premiumState(id);
            return <ReactionChoice key={id} id={id} state={state}
                                   disabled={!connected || coolingDown || state === 'loading' ||
                                     state === 'unavailable' || (definition.targeted && !hasOpponents)}
                                   onChoose={chooseReaction}/>;
          })}
        </div>
      </div>}
    </aside>
    {onFavoriteReactionsChangeAction && favoritesOpen && <ReactionFavoritesDialog
      key={favorites.join(':') || 'no-favorites'} open favorites={favorites}
      owned={owned} saving={favoritesSaving} onOpenChangeAction={setFavoritesOpen}
      onSaveAction={onFavoriteReactionsChangeAction}/>}
  </>;
}
