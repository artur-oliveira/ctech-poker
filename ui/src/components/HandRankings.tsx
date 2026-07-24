import {PlayingCard} from '@/components/table/PlayingCard';
import {HAND_RANKINGS} from '@/lib/pokerRules';

/** Shared between /poker-rules (full reference) and the table's in-game "?"
 * dialog (`compact`) — one ranked list, not two copies to drift apart. */
export function HandRankings({compact = false}: { compact?: boolean }) {
  return <ol className={`hand-ranking-list${compact ? ' compact' : ''}`}>
    {HAND_RANKINGS.map((hand, i) => <li key={hand.key}>
      <b>{i + 1}</b>
      <span className="hand-ranking-cards" aria-hidden="true">
        {hand.example.map((card, index) => (
          <PlayingCard key={card} card={card} index={index} size="hole"/>
        ))}
      </span>
      <span><strong>{hand.label}</strong><small>{hand.description}</small></span>
    </li>)}
  </ol>;
}
