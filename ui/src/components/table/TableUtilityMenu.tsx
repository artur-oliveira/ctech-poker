'use client';

import {
  CircleHelp,
  EllipsisVertical,
  Lightbulb,
  Settings2,
  Share2,
  Trophy
} from 'lucide-react';
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';

export type TableUtility = 'rankings' | 'chat' | 'reactions' | 'winners' | 'equity' | 'preferences' | 'share';

const UTILITIES = [
  {id: 'rankings', label: 'Ranking de mãos', icon: CircleHelp},
  {id: 'winners', label: 'Últimos vencedores', icon: Trophy},
  {id: 'equity', label: 'Treinador', icon: Lightbulb},
  {id: 'preferences', label: 'Preferências da mesa', icon: Settings2},
  {id: 'share', label: 'Convidar para a mesa', icon: Share2},
] as const;

export function TableUtilityMenu({active, winnersAvailable, equityTrainerVisible = false,
                                   equityTrainerAvailable = true, inviteAvailable = false, onSelectAction}: {
  active: TableUtility | null;
  winnersAvailable: boolean;
  // Gates whether the entry appears at all (sandbox room + preference on),
  // distinct from equityTrainerAvailable below, which only disables it
  // mid-decision — the entry itself should never vanish while a player is
  // deciding whether to open it.
  equityTrainerVisible?: boolean;
  // False while it's the viewer's own turn to act, so the trainer can never
  // read as a live solver assisting an in-progress decision.
  equityTrainerAvailable?: boolean;
  inviteAvailable?: boolean;
  onSelectAction: (utility: TableUtility) => void;
}) {
  const [open, setOpen] = useState(false);
  const utilities = UTILITIES.filter(utility =>
    (utility.id !== 'equity' || equityTrainerVisible) && (utility.id !== 'share' || inviteAvailable));
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<Button type="button" variant="ghost" size="icon" className="table-utility-trigger"
                                      aria-label="Mais ações da mesa"/>}>
        <EllipsisVertical aria-hidden="true"/>
      </PopoverTrigger>
      <PopoverContent className="table-utility-menu" side="bottom" align="end" aria-label="Mais ações da mesa">
        {utilities.map(({id, label, icon: Icon}) => <Button key={id} type="button" variant="ghost"
                                                            disabled={(id === 'winners' && !winnersAvailable) || (id === 'equity' && !equityTrainerAvailable)}
                                                            aria-pressed={active === id}
                                                            onClick={() => { onSelectAction(id); setOpen(false); }}>
          <Icon aria-hidden="true"/>{label}
        </Button>)}
      </PopoverContent>
    </Popover>
  );
}
