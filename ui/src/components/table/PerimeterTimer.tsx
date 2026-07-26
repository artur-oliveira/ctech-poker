import type {CSSProperties, Key} from 'react';

export function PerimeterTimer({
                                 className,
                                 durationMs,
                                 restartKey,
                                 radius
                               }: {
  className: string;
  durationMs: number;
  restartKey: Key;
  radius: number;
}) {
  return (
    <svg key={restartKey}
         className={`perimeter-timer ${className}`}
         style={{animationDuration: `${durationMs}ms`} as CSSProperties}
         aria-hidden="true"
         focusable="false">
      <rect pathLength="100" width="100%" height="100%" rx={radius} ry={radius}/>
    </svg>
  );
}
