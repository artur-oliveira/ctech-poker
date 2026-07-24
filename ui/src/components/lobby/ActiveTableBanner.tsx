'use client';
import {useQuery} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ArrowRight, Undo2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {getSessions} from '@/lib/api/player';

export function ActiveTableBanner() {
  const router = useRouter();
  const {data: sessions = []} = useQuery({queryKey: ['sessions', 'me'], queryFn: getSessions});
  const open = sessions.find(s => s.ended_at === 0);
  if (!open) return null;

  return <Button variant="ghost" className="room-card h-auto"
                 onClick={() => router.push(`/table?id=${encodeURIComponent(open.table_id)}`)}>
    <span className="status-dot"/>
    <div>
      <small>MESA EM ANDAMENTO</small>
      <h3><Undo2 size={20}/> Voltar à mesa</h3>
      <span>Você ainda está sentado — retome de onde parou.</span>
    </div>
    <ArrowRight/>
  </Button>;
}
