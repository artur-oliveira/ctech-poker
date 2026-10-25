import {LoadingRegion, Skeleton} from '@aoctech/ui';

// Not the package's StatCardsSkeleton: it draws its own card geometry, and this
// one must mirror the real `.stat-card` (label above value) so the summary bar
// keeps its height and the page below never shifts when the numbers land.
export function StatsBarSkeleton({label, count}: { label: string; count: number }) {
  return <LoadingRegion label={label} className="achievements-stats-bar">
    {Array.from({length: count}, (_, i) => <div className="stat-card" key={i}>
      <Skeleton className="skeleton-stat-label" style={{animationDelay: `${i * 90}ms`}}/>
      <Skeleton className="skeleton-stat-value" style={{animationDelay: `${i * 90}ms`}}/>
    </div>)}
  </LoadingRegion>;
}
