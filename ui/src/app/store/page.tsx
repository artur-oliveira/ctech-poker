'use client';
import {useCallback, useState} from 'react';
import Link from 'next/link';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, Club, History, ShoppingBag, Trophy} from 'lucide-react';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {TermsGate} from '@/components/TermsGate';
import {FilterGroup} from '@/components/FilterGroup';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {
  createPurchase, listPurchases, listSkus, refundPurchase, type SandboxPurchase, type SandboxSKU
} from '@/lib/api/wallet';
import {pushNotification} from '@/lib/notify';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';

type StoreTab = 'rewards' | 'purchases';

export default function Store() {
  useLobbyRealtime();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<StoreTab>('rewards');
  const [activePurchase, setActivePurchase] = useState<SandboxPurchase | null>(null);
  const [pendingSku, setPendingSku] = useState<string | null>(null);
  const [refundingId, setRefundingId] = useState<string | null>(null);

  const skus = useQuery({queryKey: ['wallet', 'skus'], queryFn: listSkus, enabled: tab === 'purchases'});
  const purchases = useQuery({
    queryKey: ['wallet', 'sandbox-purchases'], queryFn: listPurchases, enabled: tab === 'purchases'
  });

  const selectSku = useCallback(async (sku: SandboxSKU) => {
    setPendingSku(sku.id);
    try {
      const purchase = await createPurchase(sku.id);
      setActivePurchase(purchase);
      void queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
    } catch {
      pushNotification('Não foi possível iniciar a compra agora. Tente novamente.');
    } finally {
      setPendingSku(null);
    }
  }, [queryClient]);

  const refund = useCallback(async (purchaseId: string) => {
    setRefundingId(purchaseId);
    try {
      await refundPurchase(purchaseId);
      void queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
      void queryClient.invalidateQueries({queryKey: ['wallet', 'balance']});
      pushNotification('Compra estornada.', 'info');
    } catch {
      pushNotification('Não foi possível estornar esta compra agora.');
    } finally {
      setRefundingId(null);
    }
  }, [queryClient]);

  return <TermsGate>
    <main className="app-page">
      <nav className="app-nav shell">
        <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        <div className="header-right">
          <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
          <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
          <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
          <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
          <ProfileMenu/>
        </div>
      </nav>
      <section className="store shell">
        <Link href="/lobby"><ChevronLeft/> Lobby</Link>
        <header>
          <small>FICHAS SANDBOX</small>
          <ShoppingBag aria-hidden="true"/>
          <h1>Loja</h1>
          <p>Resgate sua recompensa diária ou compre fichas sandbox extras via Pix.</p>
        </header>

        <FilterGroup
          label="Seção da loja"
          value={tab}
          options={[
            {value: 'rewards', label: 'Recompensas'},
            {value: 'purchases', label: 'Compras'},
          ]}
          onChange={setTab}
        />

        {tab === 'rewards'
          ? <DailyRewardPanel/>
          : <div className="store-panel">
            <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetry={() => void skus.refetch()} onSelect={sku => void selectSku(sku)}
                     pendingSku={pendingSku}/>
            <PurchaseHistoryList purchases={purchases.data ?? []} isLoading={purchases.isLoading}
                                  isError={purchases.isError} onRetry={() => void purchases.refetch()}
                                  onRefund={id => void refund(id)} refundingId={refundingId}/>
          </div>}
      </section>
    </main>
    <PurchaseModal purchase={activePurchase} onClose={() => setActivePurchase(null)} onUpdate={setActivePurchase}/>
  </TermsGate>;
}
