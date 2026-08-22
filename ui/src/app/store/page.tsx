'use client';
import {useCallback, useRef, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronRight, Clock3, Coins, Layers, Palette, ShoppingBag, Sparkles} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {RefundConfirmationDialog} from '@/components/store/RefundConfirmationDialog';
import {ReactionPurchaseHistory, ReactionStoreSection} from '@/components/reactions/ReactionStoreSection';
import {ReactionPurchaseDialog} from '@/components/reactions/ReactionPurchaseDialog';
import {ReactionRefundDialog} from '@/components/reactions/ReactionRefundDialog';
import {DeckStoreSection, FeltStoreSection} from '@/components/store/CosmeticStoreSection';
import {CosmeticPurchaseDialog} from '@/components/store/CosmeticPurchaseDialog';
import {CosmeticRefundDialog} from '@/components/store/CosmeticRefundDialog';
import {
  createPurchase, getPurchase, listPurchases, listSkus, refundPurchase, type SandboxPurchase, type SandboxSKU
} from '@/lib/api/wallet';
import {pushNotification} from '@/lib/notify';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {getMe} from '@/lib/api/player';
import {
  getReactionPurchase, listReactionCatalog, listReactionPurchases, refundReactionPurchase,
  type ReactionCatalogEntry, type ReactionPurchase
} from '@/lib/api/reactionPurchases';
import {
  type CosmeticCatalogEntry, getCosmeticPurchase, listCosmeticCatalog, listCosmeticPurchases,
  ownedCosmeticIDs, refundCosmeticPurchase, type CosmeticPurchase
} from '@/lib/api/cosmeticPurchases';

export default function Store() {
  const queryClient = useQueryClient();
  const [activePurchase, setActivePurchase] = useState<SandboxPurchase | null>(null);
  const [pendingSku, setPendingSku] = useState<string | null>(null);
  const [refundPurchaseTarget, setRefundPurchaseTarget] = useState<SandboxPurchase | null>(null);
  const [resumingId, setResumingId] = useState<string | null>(null);
  const [reactionTarget, setReactionTarget] = useState<ReactionCatalogEntry | null>(null);
  const [activeReactionPurchase, setActiveReactionPurchase] = useState<ReactionPurchase | undefined>();
  const [reactionRefundTarget, setReactionRefundTarget] = useState<ReactionPurchase | null>(null);
  const [deckTarget, setDeckTarget] = useState<CosmeticCatalogEntry | null>(null);
  const [activeDeckPurchase, setActiveDeckPurchase] = useState<CosmeticPurchase | undefined>();
  const [deckRefundTarget, setDeckRefundTarget] = useState<CosmeticPurchase | null>(null);
  const [feltTarget, setFeltTarget] = useState<CosmeticCatalogEntry | null>(null);
  const [activeFeltPurchase, setActiveFeltPurchase] = useState<CosmeticPurchase | undefined>();
  const [feltRefundTarget, setFeltRefundTarget] = useState<CosmeticPurchase | null>(null);
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
  const deckCatalog = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'deck'], queryFn: () => listCosmeticCatalog('deck'), retry: 1,
  });
  const deckPurchases = useQuery({
    queryKey: ['wallet', 'cosmetic-purchases', 'deck'], queryFn: () => listCosmeticPurchases('deck'), retry: 1,
  });
  const feltCatalog = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'felt'], queryFn: () => listCosmeticCatalog('felt'), retry: 1,
  });
  const feltPurchases = useQuery({
    queryKey: ['wallet', 'cosmetic-purchases', 'felt'], queryFn: () => listCosmeticPurchases('felt'), retry: 1,
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

  const closePurchase = useCallback(() => setActivePurchase(null), []);

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

  const refundDeck = useCallback(async (purchaseId: string) => {
    await refundCosmeticPurchase('deck', purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: ['wallet', 'cosmetic-purchases', 'deck']}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  const refundFelt = useCallback(async (purchaseId: string) => {
    await refundCosmeticPurchase('felt', purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: ['wallet', 'cosmetic-purchases', 'felt']}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  const premiumReactionCount = (reactionCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedReactionCount = new Set((reactionPurchases.data ?? [])
    .filter(purchase => purchase.status === 'confirmed')
    .map(purchase => purchase.reaction_id)).size;
  const premiumDeckCount = (deckCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedDeckCount = ownedCosmeticIDs(deckPurchases.data ?? []).size;
  const premiumFeltCount = (feltCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedFeltCount = ownedCosmeticIDs(feltPurchases.data ?? []).size;
  const sandboxBalance = player.data?.sandbox_balance;

  return <TermsGate>
    <AppPage authed current="store">
      <AppPageBody className="store">
        <AppPageHeader
          icon={ShoppingBag}
          eyebrow="REAÇÕES E FICHAS"
          title="Loja"
          description="Personalize suas jogadas com reações permanentes ou prepare seu saldo sandbox para a próxima mesa."
        />

        <nav className="store-directory" aria-label="Seções da loja">
          <a href="#reactions">
            <span className="store-directory-icon"><Sparkles aria-hidden="true"/></span>
            <span><b>Reações</b><small>{reactionCatalog.isLoading ? 'Carregando catálogo…'
              : `${ownedReactionCount} de ${premiumReactionCount} liberadas`}</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
          <a href="#decks">
            <span className="store-directory-icon"><Layers aria-hidden="true"/></span>
            <span><b>Baralhos</b><small>{deckCatalog.isLoading ? 'Carregando catálogo…'
              : `${ownedDeckCount} de ${premiumDeckCount} liberados`}</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
          <a href="#felt">
            <span className="store-directory-icon"><Palette aria-hidden="true"/></span>
            <span><b>Feltro</b><small>{feltCatalog.isLoading ? 'Carregando catálogo…'
              : `${ownedFeltCount} de ${premiumFeltCount} liberados`}</small></span>
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

          <section id="decks" className="store-section store-department cosmetic-store-section"
                   aria-labelledby="premium-decks-title">
            <div className="store-section-heading">
              <Layers aria-hidden="true"/>
              <div><h2 id="premium-decks-title">Baralhos</h2>
                <p>Personalize as cores do baralho. Libere uma vez, use para sempre em qualquer mesa.</p></div>
            </div>
            <DeckStoreSection catalog={deckCatalog.data ?? []} purchases={deckPurchases.data ?? []}
              isLoading={deckCatalog.isLoading || deckPurchases.isLoading}
              isError={deckCatalog.isError || deckPurchases.isError}
              onRetryAction={() => {
                void deckCatalog.refetch();
                void deckPurchases.refetch();
              }}
              onBuyAction={entry => {
                setActiveDeckPurchase(undefined);
                setDeckTarget(entry);
              }}
              onResumeAction={async (entry, purchase) => {
                setDeckTarget(entry);
                try {
                  setActiveDeckPurchase(await getCosmeticPurchase('deck', purchase.purchase_id));
                } catch {
                  setActiveDeckPurchase(purchase);
                }
              }}
              onRefundAction={setDeckRefundTarget}/>
          </section>

          <section id="felt" className="store-section store-department cosmetic-store-section"
                   aria-labelledby="premium-felt-title">
            <div className="store-section-heading">
              <Palette aria-hidden="true"/>
              <div><h2 id="premium-felt-title">Feltro</h2>
                <p>Escolha o tema da mesa. Libere uma vez, use para sempre em qualquer mesa.</p></div>
            </div>
            <FeltStoreSection catalog={feltCatalog.data ?? []} purchases={feltPurchases.data ?? []}
              isLoading={feltCatalog.isLoading || feltPurchases.isLoading}
              isError={feltCatalog.isError || feltPurchases.isError}
              onRetryAction={() => {
                void feltCatalog.refetch();
                void feltPurchases.refetch();
              }}
              onBuyAction={entry => {
                setActiveFeltPurchase(undefined);
                setFeltTarget(entry);
              }}
              onResumeAction={async (entry, purchase) => {
                setFeltTarget(entry);
                try {
                  setActiveFeltPurchase(await getCosmeticPurchase('felt', purchase.purchase_id));
                } catch {
                  setActiveFeltPurchase(purchase);
                }
              }}
              onRefundAction={setFeltRefundTarget}/>
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
                   finalFocusRef={purchaseTriggerRef}
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
    <CosmeticPurchaseDialog key={`deck:${deckTarget?.id ?? 'closed'}:${activeDeckPurchase?.purchase_id ?? 'new'}`}
                            kind="deck" entry={deckTarget} initialPurchase={activeDeckPurchase}
                            sandboxBalance={player.data?.sandbox_balance}
                            onCloseAction={() => {
                              setDeckTarget(null);
                              setActiveDeckPurchase(undefined);
                            }}/>
    <CosmeticRefundDialog key={deckRefundTarget?.purchase_id ?? 'closed-deck-refund'} kind="deck"
                          purchase={deckRefundTarget} onCloseAction={() => setDeckRefundTarget(null)}
                          onConfirmAction={refundDeck}/>
    <CosmeticPurchaseDialog key={`felt:${feltTarget?.id ?? 'closed'}:${activeFeltPurchase?.purchase_id ?? 'new'}`}
                            kind="felt" entry={feltTarget} initialPurchase={activeFeltPurchase}
                            sandboxBalance={player.data?.sandbox_balance}
                            onCloseAction={() => {
                              setFeltTarget(null);
                              setActiveFeltPurchase(undefined);
                            }}/>
    <CosmeticRefundDialog key={feltRefundTarget?.purchase_id ?? 'closed-felt-refund'} kind="felt"
                          purchase={feltRefundTarget} onCloseAction={() => setFeltRefundTarget(null)}
                          onConfirmAction={refundFelt}/>
  </TermsGate>;
}
