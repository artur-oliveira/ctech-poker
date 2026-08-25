'use client';
import {useEffect, useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Eye} from 'lucide-react';
import {getHandRevealWinner, revealHandWinner} from '@/lib/api/player';

// Buy button for a history/replay view — the REST-driven counterpart to
// components/table/WinnerCards.tsx, which does the same purchase live over
// WS. Renders nothing until the check query resolves; a 404 (no archive —
// showdown/real-money/pre-feature hand) or an already-visible winner hand
// both mean there is nothing to sell.
export function RevealWinnerButton({handId, winnerName, alreadyRevealed, onRevealedAction}: {
  handId: string;
  winnerName?: string;
  alreadyRevealed: boolean;
  onRevealedAction: (cards: [string, string]) => void;
}) {
  const [pending, setPending] = useState(false);
  const check = useQuery({
    queryKey: ['hand-reveal-winner', handId],
    queryFn: () => getHandRevealWinner(handId),
  });

  useEffect(() => {
    if (check.data?.already_paid && check.data.cards) {
      onRevealedAction(check.data.cards);
    }
  }, [check.data, onRevealedAction]);

  if (alreadyRevealed || !check.data || check.data.already_paid) return null;

  const handleClick = async () => {
    setPending(true);
    try {
      const result = await revealHandWinner(handId);
      onRevealedAction(result.cards);
    } finally {
      setPending(false);
    }
  };

  return <aside className="winner-cards" aria-live="polite">
    <button type="button" disabled={pending} onClick={handleClick}>
      <Eye aria-hidden="true"/>
      <span><b>{`Ver a mão de ${winnerName || 'vencedor'} por ${check.data.fee.toLocaleString('pt-BR')} fichas`}</b>
        <small>As cartas aparecem apenas para você</small></span>
    </button>
  </aside>;
}
