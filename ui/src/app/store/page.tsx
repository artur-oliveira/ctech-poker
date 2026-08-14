'use client';
import {useCallback, useRef, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronRight, Clock3, Coins, ShoppingBag, Sparkles} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {RefundConfirmationDialog} from '@/components/store/RefundConfirmationDialog';
import {ReactionPurchaseHistory, ReactionStoreSection} from '@/components/reactions/ReactionStoreSection';
import {ReactionPurchaseDialog} from '@/components/reactions/ReactionPurchaseDialog';
import {ReactionRefundDialog} from '@/components/reactions/ReactionRefundDialog';
import {
  createPurchase, getPurchase, listPurchases, listSkus, refundPurchase, type SandboxPurchase, type SandboxSKU
} from '@/lib/api/wallet';
import {pushNotification} from '@/lib/notify';
import {useLobbyRealtime} from '@/lib/hooks/useLobbyRealtime';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {getMe} from '@/lib/api/player';
import {
  getReactionPurchase, listReactionCatalog, listReactionPurchases, refundReactionPurchase,
  type ReactionCatalogEntry, type ReactionPurchase
} from '@/lib/api/reactionPurchases';

export default function Store() {
  useLobbyRealtime();
  const queryClient = useQueryClient();
  const [activePurchase, setActivePurchase] = useState<SandboxPurchase | null>(null);
  const [pendingSku, setPendingSku] = useState<string | null>(null);
  const [refundPurchaseTarget, setRefundPurchaseTarget] = useState<SandboxPurchase | null>(null);
  const [resumingId, setResumingId] = useState<string | null>(null);
  const [reactionTarget, setReactionTarget] = useState<ReactionCatalogEntry | null>(null);
  const [activeReactionPurchase, setActiveReactionPurchase] = useState<ReactionPurchase | undefined>();
  const [reactionRefundTarget, setReactionRefundTarget] = useState<ReactionPurchase | null>(null);
  const purchaseTriggerRef = useRef<HTMLButtonElement | null>(null);
  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe});

  const skus = useQuery({
    queryKey: ['wallet', 'skus'], queryFn: listSkus, retry: 1,
  });
  const purchases = useQuery({
    queryKey: ['wallet', 'sandbox-purchases'], queryFn: listPurchases, retry: 1,
  });
  const reactionCatalog = useQuery({
    queryKey: ['wallet', 'reaction-catalog'], queryFn: listReactionCatalog, retry: 1,
  });
  const reactionPurchases = useQuery({
    queryKey: ['wallet', 'reaction-purchases'], queryFn: listReactionPurchases, retry: 1,
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

  const closePurchase = useCallback(() => {
    setActivePurchase(null);
    window.requestAnimationFrame(() => purchaseTriggerRef.current?.focus());
  }, []);

  const regeneratePurchase = useCallback(async (sku: string) => {
    const purchase = await createPurchase(sku);
    setActivePurchase(purchase);
    void queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
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

  const refundReaction = useCallback(async (purchaseId: string) => {
    await refundReactionPurchase(purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: ['wallet', 'reaction-purchases']}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  const premiumReactionCount = (reactionCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedReactionCount = new Set((reactionPurchases.data ?? [])
    .filter(purchase => purchase.status === 'confirmed')
    .map(purchase => purchase.reaction_id)).size;
  const sandboxBalance = player.data?.sandbox_balance;

  return <TermsGate>
    <AppPage authed current="store">
      <AppPageBody className="store">
        <AppPageHeader
          icon={ShoppingBag}
          eyebrow="REAÇÕES E FICHAS"
          title="Loja"
          description="Personalize suas jogadas com reações permanentes ou prepare seu saldo sandbox para a próxima mesa."
          backHref="/lobby"
        />

        <nav className="store-directory" aria-label="Seções da loja">
          <a href="#reactions">
            <span className="store-directory-icon"><Sparkles aria-hidden="true"/></span>
            <span><b>Reações</b><small>{reactionCatalog.isLoading ? 'Carregando catálogo…'
              : `${ownedReactionCount} de ${premiumReactionCount} liberadas`}</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
          <a href="#chips">
            <span className="store-directory-icon"><Coins aria-hidden="true"/></span>
            <span><b>Fichas sandbox</b><small>{sandboxBalance === undefined ? 'Carregando saldo…'
              : `${sandboxBalance.toLocaleString('pt-BR')} disponíveis`}</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
          <a href="#activity">
            <span className="store-directory-icon"><Clock3 aria-hidden="true"/></span>
            <span><b>Compras e estornos</b><small>Acompanhe toda a atividade</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
        </nav>

        <div className="store-panel">
          <section id="reactions" className="store-section store-department reaction-store-section"
                   aria-labelledby="premium-reactions-title">
            <div className="store-section-heading">
              <Sparkles aria-hidden="true"/>
              <div><h2 id="premium-reactions-title">Reações premium</h2>
                <p>Libere uma vez, use para sempre. Pague com Pix ou fichas e escolha até três atalhos para a mesa.</p></div>
            </div>
            <ReactionStoreSection catalog={reactionCatalog.data ?? []} purchases={reactionPurchases.data ?? []}
              isLoading={reactionCatalog.isLoading || reactionPurchases.isLoading}
              isError={reactionCatalog.isError || reactionPurchases.isError}
              onRetryAction={() => {
                void reactionCatalog.refetch();
                void reactionPurchases.refetch();
              }}
              onBuyAction={entry => {
                setActiveReactionPurchase(undefined);
                setReactionTarget(entry);
              }}
              onResumeAction={async (entry, purchase) => {
                setReactionTarget(entry);
                try {
                  setActiveReactionPurchase(await getReactionPurchase(purchase.purchase_id));
                } catch {
                  setActiveReactionPurchase(purchase);
                }
              }}
              onRefundAction={setReactionRefundTarget}/>
            <section id="activity" className="store-department-history" aria-labelledby="reaction-activity-title">
              <h3 id="reaction-activity-title"><Clock3 aria-hidden="true"/> Compras e estornos de reações</h3>
              <ReactionPurchaseHistory purchases={reactionPurchases.data ?? []}
                isLoading={reactionPurchases.isLoading} isError={reactionPurchases.isError}
                onRetryAction={() => void reactionPurchases.refetch()}/>
            </section>
          </section>

          <section id="chips" className="store-section store-department store-chips-department"
                   aria-labelledby="sandbox-chips-title">
            <div className="store-section-heading">
              <Coins aria-hidden="true"/>
              <div><h2 id="sandbox-chips-title">Fichas sandbox</h2>
                <p>Resgate sua recompensa ou adicione saldo para buy-ins. Fichas não têm saque nem conversão em dinheiro.</p></div>
            </div>

            <div className="store-wallet" aria-label="Seu saldo sandbox">
              <span className="store-wallet-icon"><Coins aria-hidden="true"/></span>
              <span className="store-wallet-copy"><small>Seu saldo agora</small>
                <strong>{sandboxBalance === undefined ? '—' : `${sandboxBalance.toLocaleString('pt-BR')} fichas`}</strong>
              </span>
              <span className="store-wallet-note">Saldo exclusivo do modo sandbox.</span>
            </div>

            <div className="store-department-reward" aria-label="Recompensa diária"><DailyRewardPanel/></div>

            <div className="store-subsection-heading">
              <h3 id="credit-packs-title">Pacotes de fichas</h3>
              <p>Compare o total, a quantidade base e o bônus. O pagamento é feito por Pix.</p>
            </div>
            <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetryAction={() => void skus.refetch()} onSelectAction={(sku, trigger) => {
                       purchaseTriggerRef.current = trigger;
                       void selectSku(sku);
                     }}
                     pendingSku={pendingSku}/>
            <section className="store-department-history" aria-labelledby="chip-activity-title">
              <h3 id="chip-activity-title"><Clock3 aria-hidden="true"/> Compras e estornos de fichas</h3>
              <PurchaseHistoryList purchases={purchases.data ?? []} isLoading={purchases.isLoading}
                                    isError={purchases.isError} onRetryAction={() => void purchases.refetch()}
                                    onRefund={setRefundPurchaseTarget}
                                    onResume={id => void resume(id)} resumingId={resumingId}/>
            </section>
          </section>
        </div>
      </AppPageBody>
    </AppPage>
    <PurchaseModal key={activePurchase?.purchase_id ?? 'closed_purchase'} purchase={activePurchase}
                   onCloseAction={closePurchase} onUpdateAction={setActivePurchase}
                   onRegenerateAction={regeneratePurchase}/>
    <RefundConfirmationDialog key={refundPurchaseTarget?.purchase_id ?? 'closed_refund'} purchase={refundPurchaseTarget}
                              sandboxBalance={player.data?.sandbox_balance}
                              onCloseAction={() => setRefundPurchaseTarget(null)} onConfirmAction={refund}/>
    <ReactionPurchaseDialog key={`${reactionTarget?.id ?? 'closed'}:${activeReactionPurchase?.purchase_id ?? 'new'}`}
                            entry={reactionTarget} initialPurchase={activeReactionPurchase}
                            sandboxBalance={player.data?.sandbox_balance}
                            onCloseAction={() => {
                              setReactionTarget(null);
                              setActiveReactionPurchase(undefined);
                            }}/>
    <ReactionRefundDialog key={reactionRefundTarget?.purchase_id ?? 'closed-reaction-refund'}
                          purchase={reactionRefundTarget} onCloseAction={() => setReactionRefundTarget(null)}
                          onConfirmAction={refundReaction}/>
  </TermsGate>;
}
