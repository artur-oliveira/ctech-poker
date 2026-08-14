'use client';
import {useState} from 'react';
import {Check, LockKeyhole, Star} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {PREMIUM_REACTION_IDS, TABLE_REACTIONS, type TableReactionID} from '@/lib/reactions';

export function ReactionFavoritesDialog({open, favorites, owned, saving, onOpenChangeAction, onSaveAction}: {
  open: boolean;
  favorites: TableReactionID[];
  owned: Set<string>;
  saving: boolean;
  onOpenChangeAction: (open: boolean) => void;
  onSaveAction: (favorites: TableReactionID[]) => Promise<void> | void;
}) {
  const [draft, setDraft] = useState<TableReactionID[]>(favorites);

  function toggle(id: TableReactionID) {
    setDraft(current => current.includes(id) ? current.filter(item => item !== id)
      : current.length < 3 ? [...current, id] : current);
  }

  return <Dialog open={open} onOpenChange={onOpenChangeAction}>
    <DialogContent className="reaction-favorites-dialog">
      <DialogHeader>
        <DialogTitle>Atalhos de reação</DialogTitle>
        <DialogDescription>Escolha até três para aparecerem primeiro na mesa. Você pode favoritar uma premium antes de comprá-la.</DialogDescription>
      </DialogHeader>
      <p className="reaction-favorite-count" role="status">{draft.length} de 3 selecionadas</p>
      <div className="reaction-favorite-list" role="group" aria-label="Escolher reações favoritas">
        {(Object.entries(TABLE_REACTIONS) as [TableReactionID, typeof TABLE_REACTIONS[TableReactionID]][])
          .map(([id, definition]) => {
            const selected = draft.includes(id);
            const locked = PREMIUM_REACTION_IDS.has(id) && !owned.has(id);
            return <button type="button" key={id} aria-pressed={selected}
                           disabled={!selected && draft.length >= 3} onClick={() => toggle(id)}>
              <span className="reaction-favorite-glyph" aria-hidden="true">{definition.glyph}</span>
              <span>{definition.label}</span>
              {locked && <LockKeyhole aria-label="Premium bloqueada"/>}
              {selected ? <Check aria-hidden="true"/> : <Star aria-hidden="true"/>}
            </button>;
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
