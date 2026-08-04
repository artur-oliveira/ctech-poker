'use client';
import {Download} from 'lucide-react';
import {Button} from '@/components/ui/button';
import type {HandItem} from '@/lib/api/player';
import type {HandHistoryAction} from '@/lib/api/table';
import {serializeHand} from '@/lib/handExport';

export function HandExportButton({hand, actions, viewerId, actionsAvailable = true}: {
  hand: HandItem;
  actions: HandHistoryAction[];
  viewerId?: string;
  actionsAvailable?: boolean;
}) {
  function download() {
    const blob = new Blob([serializeHand(hand, actions, viewerId, actionsAvailable)], {type: 'text/plain;charset=utf-8'});
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `ctech-poker-${hand.hand_id}.txt`;
    anchor.click();
    URL.revokeObjectURL(url);
  }
  
  const label = actionsAvailable ? 'Exportar .txt' : 'Exportar resumo .txt (ações indisponíveis)';

  return <Button type="button" variant="outline" size="icon" onClick={download}
                 aria-label={label} title={label}><Download/></Button>;
}
