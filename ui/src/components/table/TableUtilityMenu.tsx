'use client';

import {CircleHelp, MessageCircle, SmilePlus, Trophy, Wrench} from 'lucide-react';
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';

export type TableUtility = 'rankings' | 'chat' | 'reactions' | 'winners';

const UTILITIES = [
  {id: 'rankings', label: 'Ranking de mãos', icon: CircleHelp},
  {id: 'chat', label: 'Chat da mesa', icon: MessageCircle},
  {id: 'reactions', label: 'Reações', icon: SmilePlus},
  {id: 'winners', label: 'Últimos vencedores', icon: Trophy},
] as const;

export function TableUtilityMenu({active, winnersAvailable, onSelectAction}: {
  active: TableUtility | null;
  winnersAvailable: boolean;
  onSelectAction: (utility: TableUtility) => void;
}) {
  const [open, setOpen] = useState(false);
  return <Popover open={open} onOpenChange={setOpen}>
    <PopoverTrigger render={<Button type="button" variant="ghost" size="icon" className="table-utility-trigger"
      aria-label="Ferramentas da mesa"/>}><Wrench aria-hidden="true"/></PopoverTrigger>
    <PopoverContent className="table-utility-menu" side="bottom" align="end" aria-label="Ferramentas da mesa">
      {UTILITIES.map(({id, label, icon: Icon}) => <Button key={id} type="button" variant="ghost"
        disabled={id === 'winners' && !winnersAvailable} aria-pressed={active === id}
        onClick={() => { onSelectAction(id); setOpen(false); }}>
        <Icon aria-hidden="true"/>{label}
      </Button>)}
    </PopoverContent>
  </Popover>;
}
