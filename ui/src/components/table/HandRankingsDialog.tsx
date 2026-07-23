'use client';
import {CircleHelp} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';
import {HandRankings} from '@/components/HandRankings';

export function HandRankingsDialog() {
  return <Dialog>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Ver ranking de mãos"/>}>
      <CircleHelp/>
    </DialogTrigger>
    <DialogContent className="max-w-lg max-h-[85vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>Ranking de mãos</DialogTitle>
        <DialogDescription>Da mais forte à mais fraca.</DialogDescription>
      </DialogHeader>
      <HandRankings compact/>
    </DialogContent>
  </Dialog>;
}
