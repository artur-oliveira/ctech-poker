'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, History} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import {HandReplayer} from '@/components/hands/HandReplayer';
import {TermsGate} from '@/components/TermsGate';
import {RecoveryState} from '@/components/RecoveryState';
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
  
  if (!tableId || !handId) return <main>
    <RecoveryState
      title="Este replay perdeu o endereço"
      description="O link não informa qual mesa e mão devemos abrir. Volte às suas mãos para escolher outra jogada."
      action={<Button render={<Link href="/hands"/>}><History/> Ver minhas mãos</Button>}/>
  </main>;
  
  if (hand.isLoading || history.isLoading) return <main className="replay-page">
    <LoadingRegion label="Preparando a mesa de replay…" className="skeleton-panel replay-skeleton">
      <Skeleton style={{height: '18px', width: '210px'}}/>
      <Skeleton style={{height: 'min(52vh, 420px)'}}/>
      <Skeleton style={{height: '46px'}}/>
    </LoadingRegion>
  </main>;
  
  if (hand.isError || history.isError || !hand.data) return <main>
    <RecoveryState
      title="Não foi possível carregar o replay"
      description="A mão pode não pertencer à sua conta ou o histórico não está mais disponível."
      action={<Button render={<Link href="/hands"/>}><History/> Ver minhas mãos</Button>}/>
  </main>;
  
  const actions = [...(history.data?.actions || [])].sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
  return <main className="replay-page">
    <h1 className="sr-only">Replay da mão {handId}</h1>
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
