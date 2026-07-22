'use client';
import {useQuery} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {listRooms, type Room} from '@/lib/api/rooms';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';

const STATUS_LABELS: Record<string, string> = {playing: 'em jogo', waiting: 'aguardando'};

function groupByStake(rooms: Room[]) {
  const groups = new Map<string, Room[]>();
  for (const room of [...rooms].sort((a, b) => a.big_blind - b.big_blind || a.small_blind - b.small_blind)) {
    const key = `${room.small_blind} / ${room.big_blind}`;
    groups.set(key, [...(groups.get(key) || []), room]);
  }
  return [...groups.entries()];
}

export function RoomList() {
  const router = useRouter();
  const {data, isLoading} = useQuery({queryKey: ['rooms'], queryFn: listRooms, refetchInterval: 5000});
  if (isLoading) return (
    <div className="lobby-empty">
      <span className="loader"/>
      Buscando mesas…
    </div>
  );
  if (!data?.length) return (
    <div className="lobby-empty">
      Nenhuma mesa encontrada. Que tal abrir uma?
    </div>
  );
  return <div className="room-groups">{groupByStake(data).map(([stake, rooms]) => (
    <section key={stake} className="room-group" aria-label={`Mesas com blinds ${stake}`}>
      <h2><span>Blinds</span> {stake}</h2>
      <div className="room-grid">{rooms.map(r => {
        const id = r.room_id || r.id!;
        return <Button variant="ghost" key={id} className="room-card h-auto"
          onClick={() => router.push(`/table?id=${encodeURIComponent(id)}`)}>
          <span className="status-dot"/>
          <div>
            <small>SANDBOX · {STATUS_LABELS[r.status] || r.status}</small>
            <h3>{r.small_blind} / {r.big_blind}</h3>
            <span><Users/> até {r.max_seats} jogadores · buy-in {r.buy_in_min.toLocaleString('pt-BR')}–{r.buy_in_max.toLocaleString('pt-BR')}</span>
          </div>
          <ArrowRight/>
        </Button>;
      })}</div>
    </section>
  ))}</div>;
}
