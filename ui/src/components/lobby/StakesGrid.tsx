'use client';
import React, {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {createRoom, listRooms, listStakes} from '@/lib/api/rooms';
import {SkeletonList} from '@/components/ui/skeleton';
import {BUY_IN_MAX_BB, BUY_IN_MIN_BB, buyInRange} from '@/lib/pokerRules';

const MAX_SEATS_OPTIONS = [[2, 'HEADS-UP'], [6, '6-MAX'], [9, 'FULL-RING']] as const;

function bucketKey(smallBlind: number, bigBlind: number, maxSeats: number) {
  return `${smallBlind}-${bigBlind}-${maxSeats}`;
}

export function StakesGrid() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [joiningKey, setJoiningKey] = useState<string | null>(null);
  const [failedKey, setFailedKey] = useState<string | null>(null);
  const [selectedStakeKey, setSelectedStakeKey] = useState<string | null>(null);
  
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
  
  async function joinOrCreate(smallBlind: number, bigBlind: number, maxSeats: number) {
    if (joiningKey) return;
    const key = bucketKey(smallBlind, bigBlind, maxSeats);
    setFailedKey(null);
    setJoiningKey(key);
    try {
      const openRoom = rooms.find(r => r.visibility === 'public' && r.small_blind === smallBlind
        && r.big_blind === bigBlind && r.max_seats === maxSeats && r.seats_taken < maxSeats);
      let id = openRoom?.room_id || openRoom?.id || '';
      if (!id) {
        const range = buyInRange(bigBlind);
        const room = await createRoom({
          visibility: 'public', small_blind: smallBlind, big_blind: bigBlind, max_seats: maxSeats,
          buy_in_min: range.min, buy_in_max: range.max
        });
        id = room.room_id || room.id || '';
        await queryClient.invalidateQueries({queryKey: ['rooms']});
      }
      if (!id) throw new Error('A API criou uma mesa sem identificador.');
      router.push(`/table?id=${encodeURIComponent(id)}`);
    } catch {
      setFailedKey(key);
    } finally {
      setJoiningKey(null);
    }
  }
  
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
          const active = rooms.filter(r => r.visibility === 'public' && r.small_blind === selectedStake.small_blind
            && r.big_blind === selectedStake.big_blind && r.max_seats === maxSeats && r.seats_taken < maxSeats).length;
          const isJoining = joiningKey === key;
          const actionLabel = active > 0 ? 'Entrar agora' : 'Criar mesa';
          const {min: buyInMin, max: buyInMax} = buyInRange(selectedStake.big_blind);
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
                Entrada sandbox: {buyInMin.toLocaleString('pt-BR')}–{buyInMax.toLocaleString('pt-BR')} fichas ({BUY_IN_MIN_BB}–{BUY_IN_MAX_BB} BB)
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
