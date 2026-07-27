'use client';
import {useEffect, useState} from 'react';
import {CircleCheck, CircleX, Eye, LoaderCircle, ShieldCheck} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {verifyDeck, type VerifyResult} from '@/lib/deckVerify';

function truncateHash(hex: string) {
  return `${hex.slice(0, 10)}…${hex.slice(-8)}`;
}

export function DeckReveal({serverSeed, commitHash}: { serverSeed: string; commitHash: string }) {
  const [result, setResult] = useState<VerifyResult | null>(null);
  const [error, setError] = useState(false);
  const [revealed, setRevealed] = useState<Set<number>>(new Set());

  useEffect(() => {
    let cancelled = false;
    verifyDeck(serverSeed, commitHash).then(r => {
      if (!cancelled) setResult(r);
    }).catch(() => !cancelled && setError(true));
    return () => {
      cancelled = true;
    };
  }, [serverSeed, commitHash]);

  function toggle(index: number) {
    setRevealed(prev => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index); else next.add(index);
      return next;
    });
  }

  return <div className="deck-reveal">
    <div className="deck-reveal-proof">
      <div className="deck-reveal-hash"><span>Commit hash (publicado antes da mão)</span><code>{commitHash}</code></div>
      <div className="deck-reveal-hash"><span>Seed do servidor (revelada após a mão)</span><code>{serverSeed}</code></div>
      {!result && !error && <p className="deck-reveal-status pending"><LoaderCircle
          className="spin"/>Recalculando o baralho a partir da seed…</p>}
      {error && <p className="deck-reveal-status mismatch"><CircleX/>Não foi possível recalcular o baralho no seu
          navegador.</p>}
      {result && <p className={`deck-reveal-status ${result.matches ? 'match' : 'mismatch'}`}>
        {result.matches ? <CircleCheck/> : <CircleX/>}
        {result.matches
          ? 'O hash recalculado bate com o commit hash publicado: o baralho não foi alterado.'
          : `Hash recalculado não confere: ${truncateHash(result.computedHash)}`}
      </p>}
    </div>
    {result && <>
        <div className="deck-reveal-actions">
            <ShieldCheck aria-hidden="true"/>
            <p>Baralho completo, na ordem embaralhada. Clique em cada carta para revelar a posição, inclusive as que
                nunca chegaram a ser mostradas na mesa.</p>
            <button type="button" className="deck-reveal-toggle-all"
                    onClick={() => setRevealed(revealed.size === 52 ? new Set() : new Set(result.deck.map((_, i) => i)))}>
                <Eye aria-hidden="true"/>{revealed.size === 52 ? 'Ocultar tudo' : 'Revelar tudo'}
            </button>
        </div>
        <div className="deck-grid">
          {result.deck.map((card, i) => <button key={i} type="button" className="deck-grid-slot"
                                                 aria-pressed={revealed.has(i)}
                                                 aria-label={revealed.has(i) ? `Posição ${i + 1}: revelada` : `Posição ${i + 1}: revelar carta`}
                                                 onClick={() => toggle(i)}>
            <PlayingCard card={revealed.has(i) ? card.code : undefined} index={0} size="hole"/>
            <small>{i + 1}</small>
          </button>)}
        </div>
    </>}
  </div>;
}
