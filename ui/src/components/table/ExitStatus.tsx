'use client';
import {DoorOpen} from 'lucide-react';
import {Button} from '@/components/ui/button';

// isViewerTurn true means the seat's own turn-countdown ring (rendered
// elsewhere on the seat) is already showing the real, deterministic
// countdown to an auto-fold — this status intentionally does not duplicate
// it with a second number, and instead borrows the design system's "Signal
// Glow" (live, personally-timed state) via data-urgent. Cancel is only
// offered while there's genuinely something to cancel: once it's their turn
// the fold is already committed by the time this reads true (SitOutForActor
// folds synchronously on the same commit that surfaces this state).
export function ExitStatus({pendingExit, isViewerTurn, onCancelAction}: {
  pendingExit: boolean;
  isViewerTurn: boolean;
  onCancelAction: () => void;
}) {
  if (!pendingExit) return null;
  return <aside className="exit-status" data-urgent={isViewerTurn || undefined} role="status" aria-live="polite">
    <DoorOpen aria-hidden="true"/>
    <span className="exit-status-label">
      {isViewerTurn ? 'Saindo — última jogada em andamento' : 'Saindo assim que a mão terminar'}
    </span>
    {!isViewerTurn && <Button type="button" variant="ghost" size="sm" onClick={onCancelAction}>
      Cancelar saída
    </Button>}
  </aside>;
}
