'use client'
import {useQuery} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {listRooms} from '@/lib/api/rooms';
import {ArrowRight, Users} from 'lucide-react'
import {Button} from '@/components/ui/button';

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
  return <div className="room-grid">{data.map(r => {
    const id = r.room_id || r.id!;
    return <Button variant="ghost" key={id} className="room-card"
                   onClick={() => router.push(`/table?id=${encodeURIComponent(id)}`)}>
      <span className="status-dot"/>
      <div>
        <small>SANDBOX · {r.status}</small>
        <h3>{r.small_blind} / {r.big_blind}</h3>
        <span><Users/> até {r.max_seats} jogadores</span>
      </div>
      <ArrowRight/>
    </Button>
  })}</div>
}
