'use client';

import {useEffect, useState} from 'react';
import {Rabbit} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {shuffleWithSeed, verifyWirePartialDeck} from '@/lib/deckVerify';
import type {TableSnapshot} from '@/lib/api/table';
import {rabbitRunout} from '@/lib/rabbitHunt';

export function RabbitHunt({snapshot, viewer}: {snapshot: TableSnapshot; viewer?: string}) {
  const [requested, setRequested] = useState(false);
  const [cards, setCards] = useState<string[]>([]);
  const [verificationFailed, setVerificationFailed] = useState(false);
  const viewerParticipated = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  const serverRunoutAvailable = Boolean(snapshot.runout_cards && snapshot.runout_cards.length > 0);
  const available = snapshot.stage === 'complete' && snapshot.won_without_showdown &&
    snapshot.board.length < 5 && (Boolean(snapshot.shuffle_server_seed_hex) || serverRunoutAvailable) && viewerParticipated;

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
        const deck = await shuffleWithSeed(snapshot.shuffle_server_seed_hex);
        const dealtPlayers = snapshot.seats.filter(seat => seat.dealt_in).length;
        if (live) setCards(rabbitRunout(deck, dealtPlayers, snapshot.board.length));
      }
    };
    void load().catch(() => {
      if (live) setVerificationFailed(true);
    });
    return () => {
      live = false;
    };
  }, [available, requested, serverRunoutAvailable, snapshot.board.length, snapshot.revealed_card_salts,
    snapshot.root_commit_hash, snapshot.runout_cards, snapshot.seats, snapshot.shuffle_server_seed_hex,
    snapshot.unrevealed_card_hashes]);

  if (!available) return null;
  return <aside className="rabbit-hunt" aria-live="polite">
    {!requested ? <button type="button" onClick={() => setRequested(true)}>
      <Rabbit aria-hidden="true"/>
      <span><b>Ver o que viria</b><small>Rabbit hunting · não altera o resultado</small></span>
    </button> : cards.length ? <>
      <span className="rabbit-hunt-label"><Rabbit aria-hidden="true"/>O runout seria</span>
      <span className="rabbit-hunt-cards">
        {cards.map((card, index) => <PlayingCard key={`${card}:${index}`} card={card} index={index} size="hole"/>)}
      </span>
    </> : verificationFailed
      ? <span className="rabbit-hunt-label">Não foi possível verificar o runout.</span>
      : <span className="rabbit-hunt-label">Verificando o baralho…</span>}
  </aside>;
}
