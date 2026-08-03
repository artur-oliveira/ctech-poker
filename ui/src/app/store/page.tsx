'use client';
import {useCallback, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Clock3, Coins, ShoppingBag} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {RefundConfirmationDialog} from '@/components/store/RefundConfirmationDialog';
import {
  createPurchase, getPurchase, listPurchases, listSkus, refundPurchase, type SandboxPurchase, type SandboxSKU
} from '@/lib/api/wallet';
import {pushNotification} from '@/lib/notify';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {getMe} from '@/lib/api/player';

export default function Store() {
  useLobbyRealtime();
  const queryClient = useQueryClient();
  const [activePurchase, setActivePurchase] = useState<SandboxPurchase | null>(null);
  const [pendingSku, setPendingSku] = useState<string | null>(null);
  const [refundPurchaseTarget, setRefundPurchaseTarget] = useState<SandboxPurchase | null>(null);
  const [resumingId, setResumingId] = useState<string | null>(null);
  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe});

  const skus = useQuery({
    queryKey: ['wallet', 'skus'], queryFn: listSkus, retry: 1,
  });
  const purchases = useQuery({
    queryKey: ['wallet', 'sandbox-purchases'], queryFn: listPurchases, retry: 1,
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
    await refundPurchase(purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']}),
      queryClient.invalidateQueries({queryKey: ['wallet', 'balance']}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
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

        <div className="store-panel">
          <section className="store-section store-reward-section" aria-label="Recompensa diária">
            <DailyRewardPanel/>
          </section>

          <section className="store-section" aria-labelledby="credit-packs-title">
            <div className="store-section-heading">
              <Coins aria-hidden="true"/>
              <div><h2 id="credit-packs-title">Escolha quanto levar para a mesa</h2>
                <p>O total já inclui o bônus. Pague pelo Pix para receber as fichas.</p></div>
            </div>
            <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetry={() => void skus.refetch()} onSelect={sku => void selectSku(sku)}
                     pendingSku={pendingSku}/>
          </section>

          <section className="store-section store-history-section" aria-labelledby="purchase-history-title">
            <div className="store-section-heading">
              <Clock3 aria-hidden="true"/>
              <div><h2 id="purchase-history-title">Compras recentes</h2>
                <p>Acompanhe pagamentos e estornos.</p></div>
            </div>
            <PurchaseHistoryList purchases={purchases.data ?? []} isLoading={purchases.isLoading}
                                  isError={purchases.isError} onRetry={() => void purchases.refetch()}
                                  onRefund={setRefundPurchaseTarget}
                                  onResume={id => void resume(id)} resumingId={resumingId}/>
          </section>
        </div>
      </AppPageBody>
    </AppPage>
    <PurchaseModal key={activePurchase?.purchase_id ?? 'closed'} purchase={activePurchase}
                   onCloseAction={() => setActivePurchase(null)} onUpdate={setActivePurchase}/>
    <RefundConfirmationDialog key={refundPurchaseTarget?.purchase_id ?? 'closed'} purchase={refundPurchaseTarget}
                              sandboxBalance={player.data?.sandbox_balance}
                              onCloseAction={() => setRefundPurchaseTarget(null)} onConfirmAction={refund}/>
  </TermsGate>;
}
