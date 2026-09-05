'use client';
import React, {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {listRoomBuckets, listStakes, type RoomBucket} from '@/lib/api/rooms';
import {SkeletonList} from '@/components/ui/skeleton';
import {BUY_IN_MAX_BB, BUY_IN_MIN_BB, buyInRange} from '@/lib/pokerRules';
import {ROOM_BUCKETS_QUERY_KEY, tableBucketHref} from '@/lib/lobbyBuckets';

const MAX_SEATS_OPTIONS = [[2, 'HEADS-UP'], [6, '6-MAX'], [9, 'FULL-RING']] as const;

function bucketKey(smallBlind: number, bigBlind: number, maxSeats: number) {
  return `${smallBlind}-${bigBlind}-${maxSeats}`;
}

// Availability comes from the server's own aggregate over the whole public
// directory — the lobby never paginates the room list to count it (#205).
function openRooms(buckets: RoomBucket[], smallBlind: number, bigBlind: number, maxSeats: number) {
  return buckets.find(bucket => bucket.small_blind === smallBlind && bucket.big_blind === bigBlind
    && bucket.max_seats === maxSeats)?.open_rooms ?? 0;
}

export function StakesGrid() {
  const router = useRouter();
  const [joiningKey, setJoiningKey] = useState<string | null>(null);
  const [selectedStakeKey, setSelectedStakeKey] = useState<string | null>(null);

  const {data: stakes = [], isLoading: stakesLoading, isError: stakesError, refetch: refetchStakes} = useQuery({
    queryKey: ['stakes'], queryFn: () => listStakes()
  });
  const {
    data: buckets = [],
    isLoading: bucketsLoading,
    isError: bucketsError,
    refetch: refetchBuckets
  } = useQuery({
    queryKey: ROOM_BUCKETS_QUERY_KEY, queryFn: () => listRoomBuckets('sandbox')
  });

  // The click resolves nothing itself: it carries the bucket to the buy-in
  // ceremony, whose confirm is a single `POST /rooms/join-or-create` — the
  // server picks or opens the table and seats the player in one round trip.
  // No candidate walk, no per-room re-read, no client-side create, and a lost
  // last-seat race is absorbed server-side instead of bouncing back here.
  function pickBucket(smallBlind: number, bigBlind: number, maxSeats: number) {
    if (joiningKey) return;
    setJoiningKey(bucketKey(smallBlind, bigBlind, maxSeats));
    router.push(tableBucketHref({smallBlind, bigBlind, maxSeats}));
  }

  if (stakesLoading || bucketsLoading) return (
    <SkeletonList label="Buscando mesas…" count={3} height={128} className="skeleton-panel"/>
  );
  if (stakesError || bucketsError) return (
    <div className="lobby-empty" role="alert">
      Não foi possível carregar as mesas agora. Nenhuma nova mesa será criada até confirmarmos as vagas disponíveis.
      <Button variant="outline" size="sm" onClick={() => void Promise.all([refetchStakes(), refetchBuckets()])}>
        Tentar novamente
      </Button>
    </div>
  );
  if (!stakes.length) return (
    <div className="lobby-empty">
      Nenhum stake disponível no momento.
    </div>
  );
  const activeStake = stakes.find(stake => buckets.some(bucket => bucket.small_blind === stake.small_blind
    && bucket.big_blind === stake.big_blind && bucket.open_rooms > 0));
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
                       onChange={() => setSelectedStakeKey(value)}/>
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
          const active = openRooms(buckets, selectedStake.small_blind, selectedStake.big_blind, maxSeats);
          const isJoining = joiningKey === key;
          const actionLabel = active > 0 ? 'Entrar agora' : 'Criar mesa';
          const {min: buyInMin, max: buyInMax} = buyInRange(selectedStake.big_blind);
          // A join in flight blocks the other two options, but with
          // `aria-disabled` rather than `disabled`: the card stays focusable and
          // says why it cannot be used, instead of greying out in silence. The
          // click guard lives in pickBucket, so activation is a no-op anyway.
          const waiting = joiningKey !== null && !isJoining;
          return <Button variant="ghost" key={key} className="room-card h-auto"
                         loading={isJoining} aria-disabled={waiting || undefined}
                         aria-label={`${displayName}, até ${maxSeats} jogadores — ${
                           isJoining ? (active > 0 ? 'entrando…' : 'criando mesa…')
                             : waiting ? 'aguarde, outra mesa está sendo aberta' : actionLabel}`}
                         style={{'--delay': `${i * 60}ms`} as React.CSSProperties}
                         onClick={() => pickBucket(selectedStake.small_blind, selectedStake.big_blind, maxSeats)}>
            {active > 0 && <span className="status-dot"/>}
            <div>
              <small>MESA SANDBOX</small>
              <b className="room-card-name">{displayName}</b>
              <span>
                <Users/>
                {active > 0 ? `${active} mesa${active > 1 ? 's' : ''} ativa${active > 1 ? 's' : ''}` : 'Nenhuma mesa ativa'} · até {maxSeats} jogadores
              </span>
              <span className="room-card-buy-in">
                Entrada sandbox: {buyInMin.toLocaleString('pt-BR')}–{buyInMax.toLocaleString('pt-BR')} fichas ({BUY_IN_MIN_BB}–{BUY_IN_MAX_BB} BB)
              </span>
              <span className="room-card-buy-in-hint">Buy-in é a quantidade de fichas que você leva para a mesa.</span>
              <strong className="room-card-action">
                {isJoining ? (active > 0 ? 'Entrando…' : 'Criando mesa…')
                  : waiting ? 'Aguarde…' : actionLabel}
                {!isJoining && !waiting && <ArrowRight aria-hidden="true"/>}
              </strong>
            </div>
          </Button>;
        })}</div>
      </fieldset>
    </section>
  </>;
}
