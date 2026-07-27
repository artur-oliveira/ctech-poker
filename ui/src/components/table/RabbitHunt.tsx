'use client';

import {useEffect, useState} from 'react';
import {Rabbit} from 'lucide-react';
import {PlayingCard} from '@/components/table/PlayingCard';
import {shuffleWithSeed} from '@/lib/deckVerify';
import type {TableSnapshot} from '@/lib/api/table';
import {rabbitRunout} from '@/lib/rabbitHunt';

export function RabbitHunt({snapshot, viewer}: {snapshot: TableSnapshot; viewer?: string}) {
  const [requested, setRequested] = useState(false);
  const [cards, setCards] = useState<string[]>([]);
  const viewerParticipated = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  const available = snapshot.stage === 'complete' && snapshot.won_without_showdown &&
    snapshot.board.length < 5 && Boolean(snapshot.shuffle_server_seed_hex) && viewerParticipated;

  useEffect(() => {
    if (!requested || !available || !snapshot.shuffle_server_seed_hex) return undefined;
    let live = true;
    void shuffleWithSeed(snapshot.shuffle_server_seed_hex).then(deck => {
      if (live) {
        const dealtPlayers = snapshot.seats.filter(seat => seat.dealt_in).length;
        setCards(rabbitRunout(deck, dealtPlayers, snapshot.board.length));
      }
    });
    return () => {
      live = false;
    };
  }, [available, requested, snapshot.board.length, snapshot.seats, snapshot.shuffle_server_seed_hex]);

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
    </> : <span className="rabbit-hunt-label">Reconstituindo o baralho…</span>}
  </aside>;
}
