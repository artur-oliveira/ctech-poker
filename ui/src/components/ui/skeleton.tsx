import type {CSSProperties, ReactNode} from 'react';

// A single shimmering placeholder bar. Purely decorative: the surrounding
// LoadingRegion is what announces the wait to assistive tech, so every
// Skeleton stays aria-hidden and never contributes a second announcement.
export function Skeleton({className, style}: { className?: string; style?: CSSProperties }) {
  return <span aria-hidden="true" className={className ? `skeleton ${className}` : 'skeleton'} style={style}/>;
}

// The one place a loading state speaks. Keeping the original sentence as
// sr-only text means the skeleton replaces the *visible* plain text without
// removing the information a screen reader relied on.
export function LoadingRegion({label, className, children}: {
  label: string;
  className?: string;
  children: ReactNode;
}) {
  return <div role="status" aria-busy="true" className={className}>
    <span className="sr-only">{label}</span>
    {children}
  </div>;
}

// Repeated rows/cards of a known height: the shape of a list that is about to
// arrive. `className` is the real container's class so gap and grid geometry
// come from the loaded layout rather than a parallel set of rules.
export function SkeletonList({label, count, height, className}: {
  label: string;
  count: number;
  height: number;
  className?: string;
}) {
  return <LoadingRegion label={label} className={className}>
    {Array.from({length: count}, (_, i) =>
      <Skeleton key={i} style={{height: `${height}px`, animationDelay: `${i * 90}ms`}}/>)}
  </LoadingRegion>;
}

// Mirrors the real `.stat-card` (label above value) so the summary bar keeps
// its height and the page below it never shifts once the numbers land.
export function StatCardsSkeleton({label, count}: { label: string; count: number }) {
  return <LoadingRegion label={label} className="achievements-stats-bar">
    {Array.from({length: count}, (_, i) => <div className="stat-card" key={i}>
      <Skeleton className="skeleton-stat-label" style={{animationDelay: `${i * 90}ms`}}/>
      <Skeleton className="skeleton-stat-value" style={{animationDelay: `${i * 90}ms`}}/>
    </div>)}
  </LoadingRegion>;
}
