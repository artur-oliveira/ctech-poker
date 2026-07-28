'use client';
import {useMemo, useState} from 'react';
import {SmilePlus, Volume2, VolumeX, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import type {SeatView} from '@/lib/api/table';
import {
  TABLE_REACTIONS,
  type TableReactionEvent,
  type TableReactionID
} from '@/lib/reactions';
import {playerName} from '@/lib/utils';

const REACTION_MUTE_KEY = 'poker:table-reactions-muted';
const REACTION_COOLDOWN_MS = 2000;

function ReactionEffect({item}: {item: TableReactionEvent}) {
  const definition = TABLE_REACTIONS[item.reactionId];
  const positionEffect = (node: HTMLSpanElement | null) => {
    if (!node) return;
    const seats = Array.from(document.querySelectorAll<HTMLElement>('.game-seat[data-player-id]'));
    const source = seats.find(node => node.dataset.playerId === item.playerId);
    const target = item.targetPlayerId ? seats.find(node => node.dataset.playerId === item.targetPlayerId) : undefined;
    if (!source || (definition.targeted && !target)) return;
    const from = source.getBoundingClientRect();
    const to = target?.getBoundingClientRect();
    const fromX = from.left + from.width / 2;
    const fromY = from.top + from.height / 2;
    node.style.setProperty('--reaction-x', `${fromX}px`);
    node.style.setProperty('--reaction-y', `${fromY}px`);
    node.style.setProperty('--reaction-dx', `${to ? to.left + to.width / 2 - fromX : 0}px`);
    node.style.setProperty('--reaction-dy', `${to ? to.top + to.height / 2 - fromY : -72}px`);
    node.style.visibility = 'visible';
  };

  return <span ref={positionEffect}
               className={`table-reaction-effect ${definition.targeted ? 'thrown' : 'emote'}`}
               role="img" aria-label={definition.label}>{definition.glyph}</span>;
}

export function TableReactions({items, seats, viewerId, connected, onSend, open, onOpenChange}: {
  items: TableReactionEvent[];
  seats: SeatView[];
  viewerId?: string;
  connected: boolean;
  onSend: (reaction: TableReactionID, targetPlayerId?: string) => boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [muted, setMuted] = useState(() =>
    typeof window !== 'undefined' && window.localStorage.getItem(REACTION_MUTE_KEY) === 'true');
  const [coolingDown, setCoolingDown] = useState(false);
  const opponents = useMemo(() => seats.filter(seat => seat.player_id !== viewerId), [seats, viewerId]);
  const [target, setTarget] = useState('');
  const selectedTarget = opponents.some(seat => seat.player_id === target) ? target : opponents[0]?.player_id || '';

  function toggleMute() {
    setMuted(value => {
      window.localStorage.setItem(REACTION_MUTE_KEY, String(!value));
      return !value;
    });
  }

  function send(reactionId: TableReactionID) {
    if (!connected || coolingDown) return;
    const definition = TABLE_REACTIONS[reactionId];
    if (definition.targeted && !selectedTarget) return;
    if (onSend(reactionId, definition.targeted ? selectedTarget : undefined)) {
      setCoolingDown(true);
      window.setTimeout(() => setCoolingDown(false), REACTION_COOLDOWN_MS);
    }
  }

  return <>
    {!muted && <div className="table-reaction-layer" aria-live="off">
      {items.map(item => <ReactionEffect key={item.id} item={item}/>)}
    </div>}
    <aside className={`table-reactions${open ? ' open' : ''}`} aria-label="Reações da mesa">
      <Button type="button" variant="ghost" size="icon" className="reaction-toggle"
              aria-label={open ? 'Fechar reações' : 'Abrir reações'}
              aria-expanded={open} onClick={() => onOpenChange(!open)}>
        {open ? <X/> : <SmilePlus/>}
      </Button>
      {open && <div className="reaction-panel">
        <header><b>Reagir</b>
          <Button type="button" variant="ghost" size="icon"
                  aria-label={muted ? 'Ativar animações de reações' : 'Silenciar animações de reações'}
                  aria-pressed={muted} onClick={toggleMute}>
            {muted ? <VolumeX/> : <Volume2/>}
          </Button>
        </header>
        <div className="reaction-quick" role="group" aria-label="Emotes rápidos">
          {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
            .filter(([, definition]) => !definition.targeted)
            .map(([id, definition]) => <button type="button" key={id} title={definition.label}
              disabled={!connected || coolingDown} onClick={() => send(id)}>
              <span aria-hidden="true">{definition.glyph}</span><span className="sr-only">{definition.label}</span>
            </button>)}
        </div>
        <label>Enviar para
          <select value={selectedTarget} onChange={event => setTarget(event.target.value)} disabled={!opponents.length}>
            {opponents.map(seat => <option key={seat.player_id} value={seat.player_id}>
              {playerName(seat.player_id, viewerId, seat.name)}
            </option>)}
          </select>
        </label>
        <div className="reaction-objects" role="group" aria-label="Objetos">
          {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
            .filter(([, definition]) => definition.targeted)
            .map(([id, definition]) => <button type="button" key={id} title={definition.label}
              disabled={!connected || !selectedTarget || coolingDown} onClick={() => send(id)}>
              <span aria-hidden="true">{definition.glyph}</span>{definition.label}
            </button>)}
        </div>
      </div>}
    </aside>
  </>;
}
