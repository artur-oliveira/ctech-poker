'use client'
import {Star} from 'lucide-react';

export function AchievementToast({unlock}: { unlock: { key: string; stars: number } | null }) {
  return unlock ? <div key={`${unlock.key}-${unlock.stars}`} className="achievement-toast"><Star/><span><small>CONQUISTA DESBLOQUEADA</small><b>{unlock.key.replaceAll('_', ' ')}</b>{'★'.repeat(unlock.stars)}</span>
  </div> : null
}
