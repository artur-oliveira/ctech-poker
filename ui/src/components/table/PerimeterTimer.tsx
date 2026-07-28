import type {CSSProperties, Key} from 'react';

export function PerimeterTimer({
                                 className,
                                 durationMs,
                                 elapsedMs = 0,
                                 restartKey,
                                 radius
                               }: {
  className: string;
  durationMs: number;
  elapsedMs?: number;
  restartKey: Key;
  radius: number;
}) {
  return (
    <svg key={restartKey}
         className={`perimeter-timer ${className}`}
         style={{
           animationDuration: `${durationMs}ms`,
           animationDelay: `${-Math.min(durationMs, Math.max(0, elapsedMs))}ms`
         } as CSSProperties}
         aria-hidden="true"
         focusable="false">
      <rect pathLength="100" width="100%" height="100%" rx={radius} ry={radius}/>
    </svg>
  );
}
