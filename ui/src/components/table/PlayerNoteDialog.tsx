'use client';
import {useState} from 'react';
import {LoaderCircle, LockKeyhole} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {
  PLAYER_NOTE_TAGS,
  type PlayerNote,
  type PlayerNoteTag,
  savePlayerNote
} from '@/lib/api/playerNotes';
import {pushNotification} from '@/lib/notify';
import {PlayerAvatar} from '@/components/ui/player-avatar';

const TAG_LABELS: Record<PlayerNoteTag, string> = {
  red: 'Vermelho',
  orange: 'Laranja',
  yellow: 'Amarelo',
  green: 'Verde',
  blue: 'Azul',
  purple: 'Roxo'
};

type Opponent = {player_id: string; name?: string; avatar_url?: string};

export function PlayerNoteDialog({
                                   opponent,
                                   existing,
                                   open,
                                   onOpenChange,
                                   onSaved
                                 }: {
  opponent: Opponent | null;
  existing?: PlayerNote;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: (note: PlayerNote | null) => void;
}) {
  const [tag, setTag] = useState<PlayerNoteTag | ''>(existing?.tag || '');
  const [text, setText] = useState(existing?.note || '');
  const [pending, setPending] = useState(false);

  const save = async () => {
    if (!opponent || pending) return;
    setPending(true);
    try {
      const result = await savePlayerNote(opponent.player_id, {tag: tag || undefined, note: text});
      onSaved('deleted' in result ? null : result);
      pushNotification('Anotação privada atualizada.', 'info');
      onOpenChange(false);
    } finally {
      setPending(false);
    }
  };

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle className="player-note-title">
          <PlayerAvatar name={opponent?.name} avatarUrl={opponent?.avatar_url} size={32} decorative/>
          Nota sobre {opponent?.name || 'jogador'}
        </DialogTitle>
        <DialogDescription className="player-note-privacy">
          <LockKeyhole aria-hidden="true"/> Só você pode ver esta anotação.
        </DialogDescription>
      </DialogHeader>
      <div className="player-note-form">
        <fieldset>
          <legend>Tag de cor</legend>
          <div className="player-note-tags">
            <button type="button" className={!tag ? 'is-selected' : ''} onClick={() => setTag('')}>
              <span className="player-note-tag-none" aria-hidden="true"/> Sem tag
            </button>
            {PLAYER_NOTE_TAGS.map(color =>
              <button type="button" key={color} className={tag === color ? 'is-selected' : ''}
                      aria-pressed={tag === color} onClick={() => setTag(color)}>
                <span className={`player-note-dot tag-${color}`} aria-hidden="true"/>{TAG_LABELS[color]}
              </button>)}
          </div>
        </fieldset>
        <div>
          <Label htmlFor="private-player-note">Anotação</Label>
          <textarea id="private-player-note" maxLength={500} value={text}
                    placeholder="Ex.: joga muitos potes, costuma apostar forte no river…"
                    onChange={event => setText(event.target.value)}/>
          <small>{text.length}/500</small>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Cancelar</Button>
        <Button type="button" disabled={pending} onClick={save}>
          {pending && <LoaderCircle className="spin" aria-hidden="true"/>} Salvar
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
