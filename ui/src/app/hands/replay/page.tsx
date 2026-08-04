'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, CircleAlert, History} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import {HandReplayer} from '@/components/hands/HandReplayer';
import {TermsGate} from '@/components/TermsGate';
import type {WalletMode} from '@/lib/api/player';
import {getHand} from '@/lib/api/player';
import {getHandHistory} from '@/lib/api/table';
import {getViewerId} from '@/lib/utils';
import {availableWalletMode} from '@/lib/capabilities';

function ReplayContent() {
  const params = useSearchParams();
  const tableId = params.get('table_id') || '';
  const handId = params.get('hand_id') || '';
  const mode: WalletMode = availableWalletMode(params.get('mode'));
  const hand = useQuery({
    queryKey: ['hand', mode, handId],
    queryFn: () => getHand(handId, mode),
    enabled: Boolean(handId)
  });
  const history = useQuery({
    queryKey: ['hand-history', tableId, handId],
    queryFn: () => getHandHistory(tableId, handId),
    enabled: Boolean(tableId && handId)
  });
  
  if (!tableId || !handId) return <div className="replay-page-error">
    <div className="replay-error-mark" aria-hidden="true"><CircleAlert/></div>
    <h1>Este replay perdeu o endereço</h1>
    <p>O link não informa qual mesa e mão devemos abrir. Volte às suas mãos para escolher outra jogada.</p>
    <Button render={<Link href="/hands"/>}><History/> Ver minhas mãos</Button>
  </div>;
  
  if (hand.isLoading || history.isLoading) return <main className="replay-page">
    <LoadingRegion label="Preparando a mesa de replay…" className="skeleton-panel replay-skeleton">
      <Skeleton style={{height: '18px', width: '210px'}}/>
      <Skeleton style={{height: 'min(52vh, 420px)'}}/>
      <Skeleton style={{height: '46px'}}/>
    </LoadingRegion>
  </main>;
  
  if (hand.isError || history.isError || !hand.data) return <div className="replay-page-error">
    <div className="replay-error-mark" aria-hidden="true"><CircleAlert/></div>
    <h1>Não foi possível carregar o replay</h1>
    <p>A mão pode não pertencer à sua conta ou o histórico não está mais disponível.</p>
    <Button render={<Link href="/hands"/>}><History/> Ver minhas mãos</Button>
  </div>;
  
  const actions = [...(history.data?.actions || [])].sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
  return <main className="replay-page">
    <nav>
      <Link
        href={`/hands/history?table_id=${encodeURIComponent(tableId)}&hand_id=${encodeURIComponent(handId)}&mode=${mode}`}>
        <ChevronLeft/> Detalhes da mão
      </Link>
    </nav>
    <HandReplayer hand={hand.data} actions={actions} viewerId={getViewerId()}/>
  </main>;
}

export default function ReplayPage() {
  return <TermsGate>
    <Suspense fallback={<main className="replay-page">
      <LoadingRegion label="Carregando replay…" className="skeleton-panel replay-skeleton">
        <Skeleton style={{height: '18px', width: '210px'}}/>
        <Skeleton style={{height: 'min(52vh, 420px)'}}/>
      </LoadingRegion>
    </main>}>
      <ReplayContent/>
    </Suspense>
  </TermsGate>;
}
