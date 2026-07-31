'use client';
import {useCallback, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Clock3, Coins, Gift, ShoppingBag} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {FilterGroup} from '@/components/FilterGroup';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {
  createPurchase, getPurchase, listPurchases, listSkus, refundPurchase, type SandboxPurchase, type SandboxSKU
} from '@/lib/api/wallet';
import {pushNotification} from '@/lib/notify';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {getMe} from '@/lib/api/player';

type StoreTab = 'rewards' | 'purchases';

export default function Store() {
  useLobbyRealtime();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<StoreTab>('rewards');
  const [activePurchase, setActivePurchase] = useState<SandboxPurchase | null>(null);
  const [pendingSku, setPendingSku] = useState<string | null>(null);
  const [refundingId, setRefundingId] = useState<string | null>(null);
  const [resumingId, setResumingId] = useState<string | null>(null);
  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe});

  const skus = useQuery({
    queryKey: ['wallet', 'skus'], queryFn: listSkus, enabled: tab === 'purchases', retry: 1,
  });
  const purchases = useQuery({
    queryKey: ['wallet', 'sandbox-purchases'], queryFn: listPurchases, enabled: tab === 'purchases', retry: 1,
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
      void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      pushNotification('Compra estornada.', 'info');
    } catch {
      pushNotification('Não foi possível estornar esta compra agora.');
    } finally {
      setRefundingId(null);
    }
  }, [queryClient]);

  const resume = useCallback(async (purchaseId: string) => {
    setResumingId(purchaseId);
    try {
      setActivePurchase(await getPurchase(purchaseId));
    } catch {
      pushNotification('Não foi possível reabrir este pagamento agora. Tente novamente.');
    } finally {
      setResumingId(null);
    }
  }, []);

  return <TermsGate>
    <AppPage authed current="store">
      <AppPageBody className="store">
        <AppPageHeader
          icon={ShoppingBag}
          eyebrow="FICHAS SANDBOX"
          title="Loja"
          description="Ganhe fichas todos os dias ou escolha um pacote via Pix. Elas servem apenas para jogar no modo sandbox."
          backHref="/lobby"
        />

        <div className="store-wallet" aria-label="Seu saldo sandbox">
          <span className="store-wallet-icon"><Coins aria-hidden="true"/></span>
          <span className="store-wallet-copy"><small>Seu saldo sandbox</small>
            <strong>{player.data?.sandbox_balance === undefined
              ? '—'
              : `${player.data.sandbox_balance.toLocaleString('pt-BR')} fichas`}</strong>
          </span>
          <span className="store-wallet-note">Só para jogar. Sem saque ou conversão em dinheiro.</span>
        </div>

        <FilterGroup
          label="Seção da loja"
          value={tab}
          options={[
            {value: 'rewards', label: 'Ganhar grátis'},
            {value: 'purchases', label: 'Comprar via Pix'},
          ]}
          onChange={setTab}
        />

        {tab === 'rewards'
          ? <section className="store-section store-reward-section" aria-labelledby="daily-reward-title">
            <div className="store-section-heading">
              <Gift aria-hidden="true"/>
              <div><h2 id="daily-reward-title">Sua visita de hoje vale fichas</h2>
                <p>Resgate uma vez por dia e volte amanhã para outra surpresa.</p></div>
            </div>
            <DailyRewardPanel/>
          </section>
          : <div className="store-panel">
            <section className="store-section" aria-labelledby="credit-packs-title">
              <div className="store-section-heading">
                <Coins aria-hidden="true"/>
                <div><h2 id="credit-packs-title">Escolha quanto levar para a mesa</h2>
                  <p>O total mostrado já inclui o bônus. Você confirma o pagamento no Pix.</p></div>
              </div>
            <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetry={() => void skus.refetch()} onSelect={sku => void selectSku(sku)}
                     pendingSku={pendingSku}/>
            </section>
            <section className="store-section store-history-section" aria-labelledby="purchase-history-title">
              <div className="store-section-heading">
                <Clock3 aria-hidden="true"/>
                <div><h2 id="purchase-history-title">Compras recentes</h2>
                  <p>Acompanhe confirmações, expirações e estornos.</p></div>
              </div>
            <PurchaseHistoryList purchases={purchases.data ?? []} isLoading={purchases.isLoading}
                                  isError={purchases.isError} onRetry={() => void purchases.refetch()}
                                  onRefund={id => void refund(id)} refundingId={refundingId}
                                  onResume={id => void resume(id)} resumingId={resumingId}/>
            </section>
          </div>}
      </AppPageBody>
    </AppPage>
    <PurchaseModal key={activePurchase?.purchase_id ?? 'closed'} purchase={activePurchase}
                   onClose={() => setActivePurchase(null)} onUpdate={setActivePurchase}/>
  </TermsGate>;
}
