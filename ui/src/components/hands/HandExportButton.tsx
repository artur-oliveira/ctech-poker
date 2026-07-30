'use client';
import {Download} from 'lucide-react';
import {Button} from '@/components/ui/button';
import type {HandItem} from '@/lib/api/player';
import type {HandHistoryAction} from '@/lib/api/table';
import {serializeHand} from '@/lib/handExport';

export function HandExportButton({hand, actions, viewerId}: {
  hand: HandItem;
  actions: HandHistoryAction[];
  viewerId?: string;
}) {
  function download() {
    const blob = new Blob([serializeHand(hand, actions, viewerId)], {type: 'text/plain;charset=utf-8'});
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `ctech-poker-${hand.hand_id}.txt`;
    anchor.click();
    URL.revokeObjectURL(url);
  }
  
  // Icon-only: exporting is a utility beside the hand, not one of its headlines.
  return <Button type="button" variant="outline" size="icon" onClick={download}
                 aria-label="Exportar .txt" title="Exportar .txt"><Download/></Button>;
}
