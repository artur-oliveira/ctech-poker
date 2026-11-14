'use client';
import {useState} from 'react';
import {Check, Crosshair, LockKeyhole, Star, UserRound} from 'lucide-react';
import {Button} from '@aoctech/ui';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {EmojiGlyph} from '@/components/ui/EmojiGlyph';
import {
  FAVORITE_KEYS, MAX_FAVORITES_PER_KIND, PREMIUM_REACTION_IDS, TABLE_REACTIONS, type TableReactionID
} from '@/lib/reactions';

const ENTRIES = Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][];

const KINDS = [
  {targeted: false, title: 'Na minha cadeira', hint: 'Teclas 3, 4 e 5', Icon: UserRound},
  {targeted: true, title: 'Mandar para alguém', hint: 'Teclas 6, 7 e 8', Icon: Crosshair}
] as const;

export function ReactionFavoritesDialog({open, favorites, owned, saving, onOpenChangeAction, onSaveAction}: {
  open: boolean;
  favorites: TableReactionID[];
  owned: Set<string>;
  saving: boolean;
  onOpenChangeAction: (open: boolean) => void;
  onSaveAction: (favorites: TableReactionID[]) => Promise<void> | void;
}) {
  const [draft, setDraft] = useState<TableReactionID[]>(favorites);
  const countOf = (targeted: boolean) => draft.filter(id => TABLE_REACTIONS[id].targeted === targeted).length;

  function toggle(id: TableReactionID) {
    const targeted = TABLE_REACTIONS[id].targeted;
    setDraft(current => current.includes(id) ? current.filter(item => item !== id)
      : countOf(targeted) < MAX_FAVORITES_PER_KIND ? [...current, id] : current);
  }

  return <Dialog open={open} onOpenChange={onOpenChangeAction}>
    <DialogContent className="reaction-favorites-dialog">
      <DialogHeader>
        <DialogTitle>Atalhos de reação</DialogTitle>
        <DialogDescription>Até três de cada tipo ficam à mão, com uma tecla por atalho. Você pode favoritar uma
          premium antes de comprá-la.</DialogDescription>
      </DialogHeader>
      <div className="reaction-favorite-columns">
        {KINDS.map(({targeted, title, hint, Icon}) => {
          const count = countOf(targeted);
          const full = count >= MAX_FAVORITES_PER_KIND;
          return <section key={title} className="reaction-favorite-column" aria-label={title}>
            <header>
              <Icon aria-hidden="true"/>
              <span><b>{title}</b><small>{hint}</small></span>
              <output aria-live="polite">{count} de {MAX_FAVORITES_PER_KIND}</output>
            </header>
            <div className="reaction-favorite-list" role="group" aria-label={`Favoritas: ${title}`}>
              {ENTRIES.filter(([, definition]) => definition.targeted === targeted).map(([id, definition]) => {
                const selected = draft.includes(id);
                const locked = PREMIUM_REACTION_IDS.has(id) && !owned.has(id);
                const key = selected
                  ? FAVORITE_KEYS[(targeted ? MAX_FAVORITES_PER_KIND : 0)
                    + draft.filter(item => TABLE_REACTIONS[item].targeted === targeted).indexOf(id)]
                  : undefined;
                return <button type="button" key={id} aria-pressed={selected}
                               disabled={!selected && full} onClick={() => toggle(id)}>
                  <span className="reaction-favorite-glyph"><EmojiGlyph glyph={definition.glyph}/></span>
                  <span>{definition.label}</span>
                  {locked && <LockKeyhole aria-label="Premium bloqueada"/>}
                  {selected ? <kbd aria-label={`Tecla ${key}`}>{key}</kbd> : <Star aria-hidden="true"/>}
                  {selected && <Check className="reaction-favorite-check" aria-hidden="true"/>}
                </button>;
              })}
            </div>
          </section>;
        })}
      </div>
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={saving} onClick={() => onOpenChangeAction(false)}>Cancelar</Button>
        <Button type="button" disabled={saving} onClick={async () => {
          await onSaveAction(draft);
          onOpenChangeAction(false);
        }}>{saving ? 'Salvando…' : 'Salvar atalhos'}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
