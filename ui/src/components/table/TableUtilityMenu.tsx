'use client';

import {
  CircleHelp,
  EllipsisVertical,
  Lightbulb,
  ListCollapse,
  Menu,
  MessageCircle,
  SmilePlus,
  Trophy,
  Wrench
} from 'lucide-react';
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';

export type TableUtility = 'rankings' | 'chat' | 'reactions' | 'winners' | 'equity';

const UTILITIES = [
  {id: 'rankings', label: 'Ranking de mãos', icon: CircleHelp},
  {id: 'chat', label: 'Chat da mesa', icon: MessageCircle},
  {id: 'reactions', label: 'Reações', icon: SmilePlus},
  {id: 'winners', label: 'Últimos vencedores', icon: Trophy},
  {id: 'equity', label: 'Treinador', icon: Lightbulb},
] as const;

export function TableUtilityMenu({active, winnersAvailable, equityTrainerVisible = false,
                                   equityTrainerAvailable = true, onSelectAction}: {
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
  onSelectAction: (utility: TableUtility) => void;
}) {
  const [open, setOpen] = useState(false);
  const utilities = UTILITIES.filter(utility => utility.id !== 'equity' || equityTrainerVisible);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<Button type="button" variant="ghost" size="icon" className="table-utility-trigger"
                                      aria-label="Ferramentas da mesa"/>}>
        <EllipsisVertical aria-hidden="true"/>
      </PopoverTrigger>
      <PopoverContent className="table-utility-menu" side="bottom" align="end" aria-label="Ferramentas da mesa">
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
