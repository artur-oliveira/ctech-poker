'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {HandReplayer} from '@/components/hands/HandReplayer';
import {TermsGate} from '@/components/TermsGate';
import {getHand} from '@/lib/api/player';
import {getHandHistory} from '@/lib/api/table';
import {getViewerId} from '@/lib/utils';
import type {WalletMode} from '@/lib/api/player';

function ReplayContent() {
  const params = useSearchParams();
  const tableId = params.get('table_id') || '';
  const handId = params.get('hand_id') || '';
  const mode: WalletMode = params.get('mode') === 'real' ? 'real' : 'sandbox';
  const hand = useQuery({queryKey: ['hand', mode, handId], queryFn: () => getHand(handId, mode), enabled: Boolean(handId)});
  const history = useQuery({
    queryKey: ['hand-history', tableId, handId],
    queryFn: () => getHandHistory(tableId, handId),
    enabled: Boolean(tableId && handId)
  });
  
  if (!tableId || !handId) return <div className="replay-page-error">
    <h1>Replay inválido</h1>
    <p>O parâmetro de mesa ou mão está ausente ou malformatado.</p>
    <Button render={<Link href="/hands"/>}>Minhas Mãos</Button>
  </div>;
  
  if (hand.isLoading || history.isLoading) return <div className="loading-screen"><span className="loader"/>Preparando a
    mesa de replay…</div>;
  
  if (hand.isError || history.isError || !hand.data) return <div className="replay-page-error">
    <h1>Não foi possível carregar o replay</h1>
    <p>A mão pode não pertencer à sua conta ou o histórico não está mais disponível.</p>
    <Button render={<Link href="/hands"/>}>Minhas Mãos</Button>
  </div>;
  
  const actions = [...(history.data?.actions || [])].sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
  return <main className="replay-page">
    <nav>
      <Link href={`/hands/history?table_id=${encodeURIComponent(tableId)}&hand_id=${encodeURIComponent(handId)}&mode=${mode}`}>
        <ChevronLeft/> Voltar para Detalhes da Mão
      </Link>
    </nav>
    <HandReplayer hand={hand.data} actions={actions} viewerId={getViewerId()}/>
  </main>;
}

export default function ReplayPage() {
  return <TermsGate>
    <Suspense fallback={<div className="loading-screen"><span className="loader"/>Carregando replay…</div>}>
      <ReplayContent/>
    </Suspense>
  </TermsGate>;
}
