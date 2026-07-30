import {PlayingCard} from '@/components/table/PlayingCard';

// A board is always five positions wide, even when the hand ended before the
// river. Undealt positions render the card back rather than an empty outline, so
// the row keeps the physical shape of a table instead of a dashed placeholder —
// dimmed and aria-hidden because a card that was never dealt is decoration, not
// information a screen reader should count.
export function BoardSlots({board}: { board?: string[] }) {
  return <>{Array.from({length: 5}, (_, i) => board?.[i]).map((card, i) => card
    ? <PlayingCard key={i} card={card} index={i} size="board"/>
    : <span key={i} className="board-slot-undealt" aria-hidden="true">
        <PlayingCard index={i} size="board"/>
      </span>)}</>;
}
