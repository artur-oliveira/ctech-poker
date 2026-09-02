'use client';
import React, {useEffect, useRef, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {createRoom, getRoom, listRooms, listStakes, type Room} from '@/lib/api/rooms';
import {SkeletonList} from '@/components/ui/skeleton';

const MAX_SEATS_OPTIONS = [[2, 'HEADS-UP'], [6, '6-MAX'], [9, 'FULL-RING']] as const;

function bucketKey(smallBlind: number, bigBlind: number, maxSeats: number) {
  return `${smallBlind}-${bigBlind}-${maxSeats}`;
}

function openCandidates(rooms: Room[], smallBlind: number, bigBlind: number, maxSeats: number) {
  return rooms.filter(r => r.visibility === 'public' && r.small_blind === smallBlind
    && r.big_blind === bigBlind && r.max_seats === maxSeats && r.seats_taken < maxSeats);
}

export function StakesGrid() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [joiningKey, setJoiningKey] = useState<string | null>(null);
  const [failedKey, setFailedKey] = useState<string | null>(null);
  const [selectedStakeKey, setSelectedStakeKey] = useState<string | null>(null);
  const retryHandledRef = useRef(false);

  const {data: stakes = [], isLoading: stakesLoading, isError: stakesError, refetch: refetchStakes} = useQuery({
    queryKey: ['stakes'], queryFn: () => listStakes()
  });
  const {
    data: rooms = [],
    isLoading: roomsLoading,
    isError: roomsError,
    refetch: refetchRooms
  } = useQuery({
    queryKey: ['rooms'], queryFn: () => listRooms()
  });

  // The `['rooms']` cache can be up to 30s stale, and a concurrent joiner can
  // fill the last open seat in that window (or between two players' clicks
  // on the same bucket). Rather than trust a cached candidate, refetch the
  // list right before navigating and re-verify each open candidate with a
  // direct read, falling through to the next candidate (or a brand-new room)
  // instead of sending the player into a seat that is already gone. See
  // docs/plans/2026-09-02-frontend-module-review/02-lobby-store-wallet.md (F-L2).
  async function resolveRoomId(smallBlind: number, bigBlind: number, maxSeats: number) {
    const fresh = await refetchRooms();
    const candidates = openCandidates(fresh.data ?? rooms, smallBlind, bigBlind, maxSeats);
    for (const candidate of candidates) {
      const id = candidate.room_id || candidate.id || '';
      if (!id) continue;
      try {
        const verified = await getRoom(id);
        if (verified.seats_taken < verified.max_seats) return id;
      } catch {
        // The room vanished or became inaccessible between the refetch above
        // and this check; try the next candidate instead of dead-ending.
      }
    }
    const room = await createRoom({
      visibility: 'public', small_blind: smallBlind, big_blind: bigBlind, max_seats: maxSeats,
      buy_in_min: bigBlind * 20, buy_in_max: bigBlind * 100
    });
    const id = room.room_id || room.id || '';
    if (!id) throw new Error('A API criou uma mesa sem identificador.');
    await queryClient.invalidateQueries({queryKey: ['rooms']});
    return id;
  }

  async function joinOrCreate(smallBlind: number, bigBlind: number, maxSeats: number) {
    if (joiningKey) return;
    const key = bucketKey(smallBlind, bigBlind, maxSeats);
    setFailedKey(null);
    setJoiningKey(key);
    try {
      const id = await resolveRoomId(smallBlind, bigBlind, maxSeats);
      router.push(`/table?id=${encodeURIComponent(id)}`);
    } catch {
      setFailedKey(key);
    } finally {
      setJoiningKey(null);
    }
  }

  // The table page bounces a room-full failure back here with a
  // `retrySmallBlind`/`retryBigBlind`/`retrySeats` query so the player isn't
  // left to redo the pick by hand; a plain `window.location.search` read
  // (rather than `useSearchParams`) avoids requiring a Suspense boundary on
  // this otherwise static route. Runs once per mount and strips the query
  // immediately so a reload or the back button never re-triggers it.
  useEffect(() => {
    if (retryHandledRef.current || stakesLoading || roomsLoading || typeof window === 'undefined') return;
    const params = new URLSearchParams(window.location.search);
    const smallBlind = Number(params.get('retrySmallBlind'));
    const bigBlind = Number(params.get('retryBigBlind'));
    const maxSeats = Number(params.get('retrySeats'));
    if (!smallBlind || !bigBlind || !maxSeats) return;
    retryHandledRef.current = true;
    router.replace('/lobby');
    // Mirrors the retried bucket into the radio selection so the busy
    // indicator below lands on the card actually being retried.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSelectedStakeKey(`${smallBlind}-${bigBlind}`);
    void joinOrCreate(smallBlind, bigBlind, maxSeats);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stakesLoading, roomsLoading]);

  if (stakesLoading || roomsLoading) return (
    <SkeletonList label="Buscando mesas…" count={3} height={128} className="skeleton-panel"/>
  );
  if (stakesError || roomsError) return (
    <div className="lobby-empty" role="alert">
      Não foi possível carregar as mesas agora. Nenhuma nova mesa será criada até confirmarmos as vagas disponíveis.
      <Button variant="outline" size="sm" onClick={() => void Promise.all([refetchStakes(), refetchRooms()])}>
        Tentar novamente
      </Button>
    </div>
  );
  if (!stakes.length) return (
    <div className="lobby-empty">
      Nenhum stake disponível no momento.
    </div>
  );
  const activeStake = stakes.find(stake => rooms.some(room => room.visibility === 'public'
    && room.small_blind === stake.small_blind && room.big_blind === stake.big_blind
    && room.seats_taken < room.max_seats));
  const selectedStake = stakes.find(stake => `${stake.small_blind}-${stake.big_blind}` === selectedStakeKey)
    ?? activeStake ?? stakes[0];
  const selectedBlindKey = `${selectedStake.small_blind}-${selectedStake.big_blind}`;

  return <>
    <section className="room-picker" aria-labelledby="room-picker-title">
      <div className="room-picker-heading">
        <div>
          <h2 id="room-picker-title">Escolha os blinds</h2>
          <p>Blinds são as apostas obrigatórias de cada mão. O primeiro número é o small blind; o segundo, o big blind.</p>
        </div>
        <fieldset className="stake-picker">
          <legend>Blinds</legend>
          <div className="stake-options">
            {stakes.map(stake => {
              const value = `${stake.small_blind}-${stake.big_blind}`;
              return <label className="stake-option" key={value}>
                <input type="radio" name="lobby-stake" value={value} checked={selectedBlindKey === value}
                       onChange={() => {
                         setSelectedStakeKey(value);
                         setFailedKey(null);
                       }}/>
                <span>{stake.small_blind.toLocaleString('pt-BR')} / {stake.big_blind.toLocaleString('pt-BR')}</span>
              </label>;
            })}
          </div>
        </fieldset>
      </div>
      <fieldset className="room-format-group">
        <legend>Agora escolha o tamanho da mesa</legend>
        <p className="room-format-hint">O tamanho define quantos jogadores podem ocupar a mesa.</p>
        <div className="stake-grid">{MAX_SEATS_OPTIONS.map((opt, i) => {
          const maxSeats = opt[0];
          const displayName = opt[1];
          const key = bucketKey(selectedStake.small_blind, selectedStake.big_blind, maxSeats);
          const active = openCandidates(rooms, selectedStake.small_blind, selectedStake.big_blind, maxSeats).length;
          const isJoining = joiningKey === key;
          const actionLabel = active > 0 ? 'Entrar agora' : 'Criar mesa';
          const buyInMin = selectedStake.big_blind * 20;
          const buyInMax = selectedStake.big_blind * 100;
          return <Button variant="ghost" key={key} className="room-card h-auto" disabled={joiningKey !== null}
                         aria-busy={joiningKey === key}
                         style={{'--delay': `${i * 60}ms`} as React.CSSProperties}
                         onClick={() => joinOrCreate(selectedStake.small_blind, selectedStake.big_blind, maxSeats)}>
            {active > 0 && <span className="status-dot"/>}
            <div>
              <small>MESA SANDBOX</small>
              <h3>{displayName}</h3>
              <span>
                <Users/>
                {active > 0 ? `${active} mesa${active > 1 ? 's' : ''} ativa${active > 1 ? 's' : ''}` : 'Nenhuma mesa ativa'} · até {maxSeats} jogadores
              </span>
              <span className="room-card-buy-in">
                Entrada sandbox: {buyInMin.toLocaleString('pt-BR')}–{buyInMax.toLocaleString('pt-BR')} fichas (20–100 BB)
              </span>
              <span className="room-card-buy-in-hint">Buy-in é a quantidade de fichas que você leva para a mesa.</span>
              <strong className="room-card-action">
                {isJoining ? (active > 0 ? 'Entrando…' : 'Criando mesa…') : actionLabel}
                {!isJoining && <ArrowRight aria-hidden="true"/>}
              </strong>
              {failedKey === key && (
                <span className="room-card-error" role="alert">
                  Não foi possível {active > 0 ? 'entrar' : 'criar a mesa'}. Tente novamente.
                </span>
              )}
            </div>
          </Button>;
        })}</div>
      </fieldset>
    </section>
  </>;
}
