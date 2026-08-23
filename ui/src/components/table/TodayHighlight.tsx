'use client';
import {useEffect, useRef} from 'react';
import {Trophy} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {getTodayHighlight} from '@/lib/api/highlights';
import {isNotFound} from '@/lib/api/client';

// Pulled out so both branches (a permanent 404 vs. a transient failure) are
// unit-testable directly, instead of relying on TanStack Query's real retry
// timers/backoff inside a component test.
export function shouldRetryHighlightFetch(count: number, err: unknown) {
  return !isNotFound(err) && count < 3;
}

// System-detected "biggest pot of the day" for this table — no player action
// required, distinct from the manual, player-initiated hand-share flow.
// Fetched once on mount; re-fetched (via invalidateQueries, not polling) the
// moment a hand this viewer was watching completes, so a bigger pot from
// this table shows up without a page reload.
export function TodayHighlight({tableId, handId, handComplete}: {
  tableId: string;
  handId?: string;
  handComplete: boolean;
}) {
  const queryClient = useQueryClient();
  const {data} = useQuery({
    queryKey: ['highlights', tableId, 'today'],
    queryFn: () => getTodayHighlight(tableId),
    retry: shouldRetryHighlightFetch,
  });
  const lastHandId = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!handComplete || !handId || handId === lastHandId.current) return;
    lastHandId.current = handId;
    queryClient.invalidateQueries({queryKey: ['highlights', tableId, 'today']});
  }, [handComplete, handId, queryClient, tableId]);

  if (!data?.pot) return null;
  const revealedText = data.revealed && data.revealed.length > 0
    ? data.revealed.map(hand => `${hand.name || 'Jogador'} ${hand.hole_cards.join('')}`).join(' vs ')
    : undefined;
  return (
    <span className="today-highlight">
      <Trophy aria-hidden="true"/>
      <span className="today-highlight-label">Maior pote de hoje</span>
      <span className="today-highlight-pot">{data.pot.toLocaleString('pt-BR')}</span>
      {revealedText && <span className="today-highlight-cards">{revealedText}</span>}
    </span>
  );
}
