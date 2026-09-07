import {CalendarClock, Layers, Medal} from 'lucide-react';
import type {ProfileMilestone} from '@/lib/api/player';

/** Copy for every mark the server can send. An unknown key (a server ahead of
 * this client) is skipped rather than rendered as a raw slug. */
const MILESTONE_LABELS: Record<string, string> = {
  veteran_1y: '1 ano de casa',
  veteran_3y: '3 anos de casa',
  hands_1k: '1.000 mãos',
  hands_10k: '10.000 mãos',
  hands_100k: '100.000 mãos',
  top100: 'Top 100',
  top10: 'Top 10'
};

const MILESTONE_ICONS = {tenure: CalendarClock, volume: Layers, ranking: Medal};

/** The mark's own figure, spelled out — a badge that only says "10.000 mãos"
 * hides that the player is at 43.700, and the categories are told apart by
 * icon and copy rather than by colour alone. */
function detail(milestone: ProfileMilestone): string {
  const value = milestone.value.toLocaleString('pt-BR');
  if (milestone.category === 'tenure') return `${value} dias de conta`;
  if (milestone.category === 'volume') return `${value} mãos jogadas`;
  return `#${value} no ranking sandbox`;
}

/** Formats an RFC3339 timestamp as "março de 2025". Returns '' for anything
 * unparseable so a malformed date renders nothing instead of "Invalid Date". */
export function memberSinceLabel(memberSince?: string): string {
  if (!memberSince) return '';
  const date = new Date(memberSince);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString('pt-BR', {month: 'long', year: 'numeric'});
}

export function ProfileMilestones({memberSince, milestones}: {
  memberSince?: string;
  milestones?: ProfileMilestone[];
}) {
  const since = memberSinceLabel(memberSince);
  const marks = (milestones || []).filter(item => MILESTONE_LABELS[item.key]);
  if (!since && marks.length === 0) return null;

  return <div className="profile-milestones">
    {since && <p className="profile-member-since">Jogando desde <b>{since}</b></p>}
    {marks.length > 0 && <ul aria-label="Marcos do perfil">
      {marks.map(milestone => {
        const Icon = MILESTONE_ICONS[milestone.category] ?? Medal;
        return <li key={milestone.key}>
          <Icon aria-hidden="true"/>
          <span>
            <b>{MILESTONE_LABELS[milestone.key]}</b>
            <small>{detail(milestone)}</small>
          </span>
        </li>;
      })}
    </ul>}
  </div>;
}
