'use client';
import {useQuery} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {Undo2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {getSessions} from '@/lib/api/player';

export function ActiveTableBanner() {
  const router = useRouter();
  const {data: sessions = []} = useQuery({queryKey: ['sessions', 'me'], queryFn: () => getSessions()});
  const open = sessions.find(s => s.ended_at === 0);
  if (!open) return null;
  
  return <section className="active-table-strip" aria-labelledby="active-table-title">
    <div className="active-table-strip-icon" aria-hidden="true">
      <Undo2/>
    </div>
    <div className="active-table-strip-copy">
      <h2 id="active-table-title">Sua mesa continua aberta</h2>
      <p>
        Você ainda está sentado · entrada de {open.buyin_amount.toLocaleString('pt-BR')} fichas sandbox
      </p>
    </div>
    <Button size="sm" onClick={() => router.push(`/table?id=${encodeURIComponent(open.table_id)}`)}>
      Retomar mesa
    </Button>
  </section>;
}
