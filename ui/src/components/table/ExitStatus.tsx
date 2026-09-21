'use client';
import {DoorOpen} from 'lucide-react';
import {Button} from '@/components/ui/button';

// isViewerTurn true means the seat's own turn-countdown ring (rendered
// elsewhere on the seat) is already showing the real, deterministic
// countdown to an auto-fold — this status intentionally does not duplicate
// it with a second number, and instead borrows the design system's "Signal
// Glow" (live, personally-timed state) via data-urgent.
//
// Cancel used to be withheld on the viewer's own turn, on the assumption that
// by then the fold was already committed (SitOutForActor folds synchronously
// on the same commit that surfaces this state). That only holds when the exit
// was requested while the player was ALREADY on the clock. Request it a beat
// earlier and hand.RequestExit deliberately leaves them alone, deferring the
// fold to the server's auto-fold sweep — and any delay in that sweep stranded
// the player with no action buttons (a pending exit takes them away) and no
// way back either, burning their turn and their whole time bank. Seen live on
// 2026-09-21. The escape hatch must stay reachable for exactly as long as the
// exit is still pending; cancelling on your own turn is well-defined
// server-side (hand.CancelExit) and never un-folds a fold already taken.
export function ExitStatus({pendingExit, isViewerTurn, onCancelAction}: {
  pendingExit: boolean;
  isViewerTurn: boolean;
  onCancelAction: () => void;
}) {
  if (!pendingExit) return null;
  return <aside className="exit-status" data-urgent={isViewerTurn || undefined} role="status" aria-live="polite">
    <DoorOpen aria-hidden="true"/>
    <span className="exit-status-label">
      {isViewerTurn ? 'Saindo: última jogada em andamento' : 'Saindo assim que a mão terminar'}
    </span>
    <Button type="button" variant="ghost" size="sm" onClick={onCancelAction}>
      Cancelar saída
    </Button>
  </aside>;
}
