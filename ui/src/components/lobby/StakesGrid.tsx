'use client';
import React, {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {createRoom, listRooms, listStakes} from '@/lib/api/rooms';
import {pushNotification} from '@/lib/notify';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';

const MAX_SEATS_OPTIONS = [[2, 'HEADS-UP'], [6, '6-MAX'], [9, 'FULL-RING']] as const;

function bucketKey(smallBlind: number, bigBlind: number, maxSeats: number) {
  return `${smallBlind}-${bigBlind}-${maxSeats}`;
}

export function StakesGrid() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [joiningKey, setJoiningKey] = useState<string | null>(null);
  
  useLobbyRealtime();
  
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
    setJoiningKey(key);
    try {
      const openRoom = rooms.find(r => r.visibility === 'public' && r.small_blind === smallBlind
        && r.big_blind === bigBlind && r.max_seats === maxSeats && r.seats_taken < maxSeats);
      let id = openRoom?.room_id || openRoom?.id || '';
      if (!id) {
        const room = await createRoom({
          visibility: 'public', small_blind: smallBlind, big_blind: bigBlind, max_seats: maxSeats,
          buy_in_min: bigBlind * 20, buy_in_max: bigBlind * 100
        });
        id = room.room_id || room.id || '';
        await queryClient.invalidateQueries({queryKey: ['rooms']});
      }
      if (!id) throw new Error('A API criou uma mesa sem identificador.');
      router.push(`/table?id=${encodeURIComponent(id)}`);
    } catch {
      pushNotification('Não foi possível entrar na mesa. Tente novamente.', 'error');
    } finally {
      setJoiningKey(null);
    }
  }
  
  if (stakesLoading || roomsLoading) return (
    <div className="lobby-empty">
      <span className="loader"/>
      Buscando mesas…
    </div>
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
  return <>
    <p className="stake-hint">
      Toque em um stake: você entra numa mesa aberta ou criamos uma na hora.
    </p>
    <div className="room-groups">{stakes.map(stake => (
      <section key={`${stake.small_blind}-${stake.big_blind}`} className="room-group"
               aria-label={`Mesas com blinds ${stake.small_blind.toLocaleString('pt-BR')} / ${stake.big_blind.toLocaleString('pt-BR')}`}>
        <h2>
          <span>Blinds </span>
          {stake.small_blind.toLocaleString('pt-BR')} / {stake.big_blind.toLocaleString('pt-BR')}
        </h2>
        <div className="stake-grid">{MAX_SEATS_OPTIONS.map((opt, i) => {
          const maxSeats = opt[0];
          const displayName = opt[1];
          const key = bucketKey(stake.small_blind, stake.big_blind, maxSeats);
          const active = rooms.filter(r => r.visibility === 'public' && r.small_blind === stake.small_blind
            && r.big_blind === stake.big_blind && r.max_seats === maxSeats && r.seats_taken < maxSeats).length;
          return <Button variant="ghost" key={key} className="room-card h-auto" disabled={joiningKey !== null}
                         aria-busy={joiningKey === key}
                         style={{'--delay': `${i * 60}ms`} as React.CSSProperties}
                         onClick={() => joinOrCreate(stake.small_blind, stake.big_blind, maxSeats)}>
            {active > 0 && <span className="status-dot"/>}
            <div>
              <small>SANDBOX · {displayName}</small>
              <h3>{stake.small_blind.toLocaleString('pt-BR')} / {stake.big_blind.toLocaleString('pt-BR')}</h3>
              <span>
                <Users/>
                {active > 0 ? `${active} mesa${active > 1 ? 's' : ''} ativa${active > 1 ? 's' : ''}` : 'Nenhuma mesa ativa'} · até {maxSeats} jogadores
              </span>
            </div>
            <ArrowRight/>
          </Button>;
        })}</div>
      </section>
    ))}
    </div>
  </>;
}
