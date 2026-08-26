'use client';

import {useEffect, useRef, useState} from 'react';
import {Rabbit} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {verifyDeck, verifyWirePartialDeck} from '@/lib/deckVerify';
import type {TableSnapshot} from '@/lib/api/table';
import {rabbitRunout} from '@/lib/rabbitHunt';

export function RabbitHunt({snapshot, viewer, bigBlind, pending, failCount, onRequestRabbitHuntAction, onRabbitHuntVerifyFailedAction}: {
  snapshot: TableSnapshot;
  viewer?: string;
  bigBlind: number;
  pending?: boolean;
  failCount?: number;
  onRequestRabbitHuntAction?: () => void;
  onRabbitHuntVerifyFailedAction?: () => void;
}) {
  const [requested, setRequested] = useState(false);
  const [cards, setCards] = useState<string[]>([]);
  const [verificationFailed, setVerificationFailed] = useState(false);
  const lastFailCount = useRef(failCount ?? 0);

  // The server rejects the request outright (e.g. "hand not complete yet"
  // while this client's snapshot is momentarily ahead of the server's)
  // rather than acking it, so no runout ever arrives. Without this,
  // `requested` stays true forever and the button never comes back for the
  // player to retry.
  useEffect(() => {
    if ((failCount ?? 0) !== lastFailCount.current) {
      lastFailCount.current = failCount ?? 0;
      setRequested(false);
    }
  }, [failCount]);
  const viewerParticipated = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  const serverRunoutAvailable = Boolean(snapshot.runout_cards && snapshot.runout_cards.length > 0);
  // The offer is gated on the deck *commitment*, never on the proof: the
  // server withholds the seed, the salts and the runout of a hand won without
  // showdown until this viewer has paid (snapshot.go's rabbitHuntPaid check),
  // so requiring them here made the button that triggers the payment depend on
  // data only the payment produces — it never rendered. Verification still
  // happens in the browser, below, once the paid snapshot arrives.
  const commitmentPublished = Boolean(snapshot.shuffle_commit_hash || snapshot.root_commit_hash ||
    snapshot.shuffle_server_seed_hex || serverRunoutAvailable);
  const available = snapshot.stage === 'complete' && snapshot.won_without_showdown &&
    snapshot.board.length < 5 && commitmentPublished && viewerParticipated;
  
  useEffect(() => {
    if (!requested || !available) return undefined;
    let live = true;
    const load = async () => {
      if (serverRunoutAvailable && snapshot.runout_cards) {
        if (!snapshot.root_commit_hash || !snapshot.revealed_card_salts || !snapshot.unrevealed_card_hashes) {
          throw new Error('missing partial deck proof');
        }
        const verification = await verifyWirePartialDeck(
          snapshot.root_commit_hash,
          snapshot.revealed_card_salts,
          snapshot.unrevealed_card_hashes
        );
        if (!verification.matches) throw new Error('invalid partial deck proof');
        if (live) setCards(snapshot.runout_cards);
      } else if (snapshot.shuffle_server_seed_hex) {
        if (!snapshot.shuffle_commit_hash) throw new Error('missing shuffle commit hash');
        const verification = await verifyDeck(snapshot.shuffle_server_seed_hex, snapshot.shuffle_commit_hash);
        if (!verification.matches) throw new Error('invalid shuffle commit hash');
        const dealtPlayers = snapshot.seats.filter(seat => seat.dealt_in).length;
        if (live) setCards(rabbitRunout(verification.deck, dealtPlayers, snapshot.board.length));
      }
    };
    void load().catch(() => {
      if (live) {
        setVerificationFailed(true);
        onRabbitHuntVerifyFailedAction?.();
      }
    });
    return () => {
      live = false;
    };
  }, [available, requested, serverRunoutAvailable, snapshot.board.length, snapshot.revealed_card_salts,
    snapshot.root_commit_hash, snapshot.runout_cards, snapshot.seats, snapshot.shuffle_commit_hash,
    snapshot.shuffle_server_seed_hex, snapshot.unrevealed_card_hashes, onRabbitHuntVerifyFailedAction]);

  if (!available) return null;
  return <aside className="rabbit-hunt" aria-live="polite">
    {!requested ? <button type="button" disabled={pending} onClick={() => {
      setRequested(true);
      onRequestRabbitHuntAction?.();
    }}>
      <Rabbit aria-hidden="true"/>
      <span><b>{`Ver por ${bigBlind.toLocaleString('pt-BR')} fichas`}</b><small>Rabbit hunting · não altera o resultado</small></span>
    </button> : cards.length ? <>
      <span className="rabbit-hunt-label"><Rabbit aria-hidden="true"/>O runout seria</span>
      <span className="rabbit-hunt-cards">
        {cards.map((card, index) => <PlayingCard key={`${card}:${index}`} card={card} index={index} size="hole"/>)}
      </span>
    </> : verificationFailed
      ? <span className="rabbit-hunt-label">Não foi possível verificar o runout. Taxa devolvida.</span>
      : <span className="rabbit-hunt-label">Verificando o baralho…</span>}
  </aside>;
}
