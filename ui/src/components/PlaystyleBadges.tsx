import {playstyleMeta, type PlaystyleBadge} from '@/lib/playstyle';

export function PlaystyleBadges({badges, className}: {
  badges: readonly PlaystyleBadge[];
  className?: string;
}) {
  return <div className={`poker-style-badges${className ? ` ${className}` : ''}`}>
    {badges.map(badge => {
      const meta = playstyleMeta(badge.key);
      if (!meta) return null;
      return <details key={badge.key} className="playstyle-disclosure">
        <summary>{meta.label}</summary>
        <p>{meta.reason}</p>
      </details>;
    })}
  </div>;
}
