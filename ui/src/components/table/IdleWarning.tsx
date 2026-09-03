'use client';
import {useEffect, useState} from 'react';
import {ClockAlert} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {useLiveNow} from '@/lib/hooks/useLiveNow';

/** The last-minute "you are about to be removed for inactivity" alert.
 *
 * Nothing ticks until the deadline is actually within a minute: a single
 * timeout waits out the quiet part, and only then does the component join the
 * shared table clock at 1 Hz. */
export function IdleWarning({deadline, onKeepSeat}: { deadline?: number; onKeepSeat: () => boolean }) {
  // Scoped to the deadline it was armed for, so a fresh deadline (the server
  // re-arms one per idle spell) starts the quiet wait over instead of
  // inheriting the previous spell's armed state.
  const [armedFor, setArmedFor] = useState<number | undefined>(undefined);
  useEffect(() => {
    if (!deadline) return undefined;
    const timeout = setTimeout(() => setArmedFor(deadline), Math.max(0, deadline - Date.now() - 60_000));
    return () => clearTimeout(timeout);
  }, [deadline]);
  const armed = Boolean(deadline) && armedFor === deadline;
  const now = useLiveNow(armed, 1000);
  if (!deadline || !armed) return null;
  const seconds = Math.max(0, Math.ceil((deadline - now) / 1000));
  if (seconds > 60) return null;
  return <div className="idle-warning" role="alert">
    <ClockAlert aria-hidden="true"/>
    <p>Você será removido por inatividade em <strong>{seconds}s</strong>.</p>
    <Button type="button" variant="outline" onClick={onKeepSeat}>Continuar na mesa</Button>
  </div>;
}
