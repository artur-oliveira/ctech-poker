'use client';
import {useEffect, useState} from 'react';
import {CircleCheck, CircleX, Eye, LoaderCircle, ShieldCheck} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {verifyWirePartialDeck, type WireCardReveal} from '@/lib/deckVerify';

// Fairness proof for a hand that ended without a full showdown: the server
// cannot publish the seed (it would expose mucked hole cards), so each deck
// position arrives either as card + salt or as its committed hash alone. Both
// hash back into root_commit_hash, so the deck is still provably unaltered —
// this is the same guarantee DeckReveal gives, minus the cards you never
// earned the right to see.
export function PartialDeckProof({rootCommitHash, revealed, unrevealed}: {
  rootCommitHash: string;
  revealed: Record<number, WireCardReveal>;
  unrevealed: Record<number, string>;
}) {
  const [matches, setMatches] = useState<boolean | null>(null);
  const [error, setError] = useState(false);
  const [flipped, setFlipped] = useState<Set<number>>(new Set());
  
  useEffect(() => {
    let cancelled = false;
    verifyWirePartialDeck(rootCommitHash, revealed, unrevealed).then(r => {
      if (!cancelled) setMatches(r.matches);
    }).catch(() => !cancelled && setError(true));
    return () => {
      cancelled = true;
    };
  }, [rootCommitHash, revealed, unrevealed]);
  
  const revealedIndexes = Object.keys(revealed).map(Number);
  
  function toggle(index: number) {
    setFlipped(prev => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index); else next.add(index);
      return next;
    });
  }
  
  return <div className="deck-reveal">
    <div className="deck-reveal-summary" aria-live="polite">
      {matches === null && !error && <p className="deck-reveal-status pending"><LoaderCircle
          className="spin"/>Recalculando os hashes das 52 posições…</p>}
      {error && <p className="deck-reveal-status mismatch"><CircleX/>Não foi possível verificar o baralho no seu
          navegador.</p>}
      {matches !== null && <p className={`deck-reveal-status ${matches ? 'match' : 'mismatch'}`}>
        {matches ? <CircleCheck/> : <CircleX/>}
        {matches
          ? 'Os hashes das 52 posições recompõem o root commit publicado: o baralho não foi alterado.'
          : 'Os hashes recalculados não conferem com o root commit publicado.'}
      </p>}
    </div>
    <details className="deck-reveal-details">
      <summary>Ver detalhes técnicos e posições permitidas</summary>
      <div className="deck-reveal-proof">
        <div className="deck-reveal-hash">
          <span>Root commit hash (publicado antes da mão)</span><code>{rootCommitHash}</code>
        </div>
      </div>
      <div className="deck-reveal-actions">
        <ShieldCheck aria-hidden="true"/>
        <p>Esta mão terminou sem showdown, então a seed do servidor não é publicada; ela revelaria cartas que ninguém
          pagou para ver. Cada posição vem como carta + salt permitido ou somente como hash comprometido.</p>
        <span className="deck-reveal-count" aria-live="polite">{flipped.size} de {revealedIndexes.length} cartas permitidas reveladas</span>
        <button type="button" className="deck-reveal-toggle-all"
                onClick={() => setFlipped(flipped.size === revealedIndexes.length ? new Set() : new Set(revealedIndexes))}>
          <Eye aria-hidden="true"/>{flipped.size === revealedIndexes.length ? 'Ocultar tudo' : 'Revelar tudo'}
        </button>
      </div>
      <div className="deck-grid">
        {Array.from({length: 52}, (_, i) => {
          const reveal = revealed[i];
          return reveal
            ? <button key={i} type="button" className="deck-grid-slot" aria-pressed={flipped.has(i)}
                      aria-label={flipped.has(i) ? `Posição ${i + 1}: revelada` : `Posição ${i + 1}: revelar carta`}
                      onClick={() => toggle(i)}>
              <PlayingCard card={flipped.has(i) ? reveal.card : undefined} index={0} size="hole"/>
              <small>{i + 1}</small>
            </button>
            : <span key={i} className="deck-grid-slot" title={unrevealed[i]}
                    aria-label={`Posição ${i + 1}: carta não revelada, apenas hash comprometido`}>
              <PlayingCard card={undefined} index={0} size="hole"/>
              <small>{i + 1}</small>
            </span>;
        })}
      </div>
    </details>
  </div>;
}
