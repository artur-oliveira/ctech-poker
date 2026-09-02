'use client';
import {useCallback, useRef, useState} from 'react';
import {useInfiniteQuery, useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronRight, Clock3, Coins, Layers, Palette, ShoppingBag, Sparkles} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {DailyRewardPanel} from '@/components/store/DailyRewardPanel';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PurchaseModal} from '@/components/store/PurchaseModal';
import {PurchaseHistoryList} from '@/components/store/PurchaseHistoryList';
import {RefundConfirmationDialog} from '@/components/store/RefundConfirmationDialog';
import {reactionActivityRows, ReactionStoreSection} from '@/components/reactions/ReactionStoreSection';
import {ReactionPurchaseDialog} from '@/components/reactions/ReactionPurchaseDialog';
import {ReactionRefundDialog} from '@/components/reactions/ReactionRefundDialog';
import {cosmeticActivityRows, DeckStoreSection, FeltStoreSection} from '@/components/store/CosmeticStoreSection';
import {PurchaseActivityList} from '@/components/store/PurchaseActivityList';
import {CosmeticPurchaseDialog} from '@/components/store/CosmeticPurchaseDialog';
import {CosmeticRefundDialog} from '@/components/store/CosmeticRefundDialog';
import {
  createPurchase,
  getPurchase,
  listPurchases,
  listSkus,
  refundPurchase,
  WALLET_QUERY_ROOT,
  type SandboxPurchase,
  type SandboxSKU
} from '@/lib/api/wallet';
import type {Page} from '@/lib/api/client';
import {pushNotification} from '@/lib/notify';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {getMe} from '@/lib/api/player';
import {
  getReactionPurchase,
  listReactionCatalog,
  listReactionPurchases,
  REACTION_PURCHASE_HISTORY_KEY,
  type ReactionCatalogEntry,
  type ReactionPurchase,
  refundReactionPurchase
} from '@/lib/api/reactionPurchases';
import {
  type CosmeticCatalogEntry,
  type CosmeticPurchase,
  getCosmeticPurchase,
  listCosmeticCatalog,
  listCosmeticPurchases,
  refundCosmeticPurchase
} from '@/lib/api/cosmeticPurchases';

// usePurchaseHistory walks a cursor-paginated purchase list and hands the page
// the flattened rows plus a "load more" affordance. Every purchase endpoint
// returns the same {data, has_next, next_cursor, …} envelope, so one hook
// covers chips, reactions and cosmetics.
function usePurchaseHistory<T>(queryKey: readonly unknown[], fetchPage: (cursor?: string) => Promise<Page<T>>) {
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({pageParam}) => fetchPage(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page: Page<T>) => (page.has_next && page.next_cursor) || undefined,
  });
  return {
    items: query.data?.pages.flatMap(page => page.data) ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    refetch: query.refetch,
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => void query.fetchNextPage(),
  };
}

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
  // One shared slot: only one dialog is ever open, and every trigger writes here
  // on the way in so focus goes back where it came from on close.
  const purchaseTriggerRef = useRef<HTMLButtonElement | null>(null);
  const openFrom = useCallback(<T,>(trigger: HTMLButtonElement, open: () => T) => {
    purchaseTriggerRef.current = trigger;
    return open();
  }, []);
  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe});

  const skus = useQuery({
    queryKey: ['wallet', 'skus'], queryFn: listSkus,
  });
  const reactionCatalog = useQuery({
    queryKey: ['wallet', 'reaction-catalog'], queryFn: listReactionCatalog,
  });
  const deckCatalog = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'deck'], queryFn: () => listCosmeticCatalog('deck'),
  });
  const feltCatalog = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'felt'], queryFn: () => listCosmeticCatalog('felt'),
  });
  // Purchase history is cursor-paginated; ownership is not read from it (see
  // ownedCosmeticIDs), so these only feed the activity lists and the
  // resume/refund actions.
  const purchases = usePurchaseHistory(['wallet', 'sandbox-purchases'], listPurchases);
  const reactionPurchases = usePurchaseHistory(REACTION_PURCHASE_HISTORY_KEY, listReactionPurchases);
  const deckPurchases = usePurchaseHistory(['wallet', 'cosmetic-purchases', 'deck'],
    cursor => listCosmeticPurchases('deck', cursor));
  const feltPurchases = usePurchaseHistory(['wallet', 'cosmetic-purchases', 'felt'],
    cursor => listCosmeticPurchases('felt', cursor));

  const selectSku = useCallback(async (sku: SandboxSKU) => {
    setPendingSku(sku.id);
    try {
      const purchase = await createPurchase(sku.id);
      setActivePurchase(purchase);
      void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
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
    void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
  }, [queryClient]);

  const refund = useCallback(async (purchaseId: string) => {
    await refundPurchase(purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT}),
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
      queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  const refundDeck = useCallback(async (purchaseId: string) => {
    await refundCosmeticPurchase('deck', purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  const refundFelt = useCallback(async (purchaseId: string) => {
    await refundCosmeticPurchase('felt', purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }, [queryClient]);

  // Both halves of every "N de M liberados" come from the same catalog
  // response, so the numerator can never outrun the denominator the way it did
  // when ownership was inferred from purchase history.
  const premiumReactionCount = (reactionCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedReactionCount = (reactionCatalog.data ?? []).filter(entry => entry.premium && entry.owned).length;
  const premiumDeckCount = (deckCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedDeckCount = (deckCatalog.data ?? []).filter(entry => entry.premium && entry.owned).length;
  const premiumFeltCount = (feltCatalog.data ?? []).filter(entry => entry.premium).length;
  const ownedFeltCount = (feltCatalog.data ?? []).filter(entry => entry.premium && entry.owned).length;
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
            <span><b>Compras e estornos</b><small>Recibos de reações e cosméticos</small></span>
            <ChevronRight aria-hidden="true"/>
          </a>
        </nav>

        <div className="store-panel">
          <section id="reactions" className="store-section store-department reaction-store-section"
                   aria-labelledby="premium-reactions-title">
            <div className="store-section-heading">
              <Sparkles aria-hidden="true"/>
              <div><h2 id="premium-reactions-title">Reações premium</h2>
                <p>Libere uma vez, use para sempre. Pague com Pix ou fichas e escolha até três atalhos para a mesa.</p>
              </div>
            </div>
            <ReactionStoreSection catalog={reactionCatalog.data ?? []} purchases={reactionPurchases.items}
                                  isLoading={reactionCatalog.isLoading || reactionPurchases.isLoading}
                                  isError={reactionCatalog.isError || reactionPurchases.isError}
                                  onRetryAction={() => {
                                    void reactionCatalog.refetch();
                                    void reactionPurchases.refetch();
                                  }}
                                  onBuyAction={(entry, trigger) => openFrom(trigger, () => {
                                    setActiveReactionPurchase(undefined);
                                    setReactionTarget(entry);
                                  })}
                                  onResumeAction={(entry, purchase, trigger) => openFrom(trigger, async () => {
                                    setReactionTarget(entry);
                                    try {
                                      setActiveReactionPurchase(await getReactionPurchase(purchase.purchase_id));
                                    } catch {
                                      setActiveReactionPurchase(purchase);
                                    }
                                  })}
                                  onRefundAction={(purchase, trigger) =>
                                    openFrom(trigger, () => setReactionRefundTarget(purchase))}/>
          </section>

          <section id="decks" className="store-section store-department cosmetic-store-section"
                   aria-labelledby="premium-decks-title">
            <div className="store-section-heading">
              <Layers aria-hidden="true"/>
              <div><h2 id="premium-decks-title">Baralhos</h2>
                <p>Personalize as cores do baralho. Libere uma vez, use para sempre em qualquer mesa.</p></div>
            </div>
            <DeckStoreSection catalog={deckCatalog.data ?? []}
                              purchases={deckPurchases.items}
                              isLoading={deckCatalog.isLoading || deckPurchases.isLoading}
                              isError={deckCatalog.isError || deckPurchases.isError}
                              onRetryAction={() => {
                                void deckCatalog.refetch();
                                void deckPurchases.refetch();
                              }}
                              onBuyAction={(entry, trigger) => openFrom(trigger, () => {
                                setActiveDeckPurchase(undefined);
                                setDeckTarget(entry);
                              })}
                              onResumeAction={(entry, purchase, trigger) => openFrom(trigger, async () => {
                                setDeckTarget(entry);
                                try {
                                  setActiveDeckPurchase(await getCosmeticPurchase('deck', purchase.purchase_id));
                                } catch {
                                  setActiveDeckPurchase(purchase);
                                }
                              })}
                              onRefundAction={(purchase, trigger) =>
                                openFrom(trigger, () => setDeckRefundTarget(purchase))}/>
          </section>

          <section id="felt" className="store-section store-department cosmetic-store-section"
                   aria-labelledby="premium-felt-title">
            <div className="store-section-heading">
              <Palette aria-hidden="true"/>
              <div><h2 id="premium-felt-title">Feltro</h2>
                <p>Escolha o tema da mesa. Libere uma vez, use para sempre em qualquer mesa.</p></div>
            </div>
            <FeltStoreSection catalog={feltCatalog.data ?? []} purchases={feltPurchases.items}
                              isLoading={feltCatalog.isLoading || feltPurchases.isLoading}
                              isError={feltCatalog.isError || feltPurchases.isError}
                              onRetryAction={() => {
                                void feltCatalog.refetch();
                                void feltPurchases.refetch();
                              }}
                              onBuyAction={(entry, trigger) => openFrom(trigger, () => {
                                setActiveFeltPurchase(undefined);
                                setFeltTarget(entry);
                              })}
                              onResumeAction={(entry, purchase, trigger) => openFrom(trigger, async () => {
                                setFeltTarget(entry);
                                try {
                                  setActiveFeltPurchase(await getCosmeticPurchase('felt', purchase.purchase_id));
                                } catch {
                                  setActiveFeltPurchase(purchase);
                                }
                              })}
                              onRefundAction={(purchase, trigger) =>
                                openFrom(trigger, () => setFeltRefundTarget(purchase))}/>
          </section>

          <section id="chips" className="store-section store-department store-chips-department"
                   aria-labelledby="sandbox-chips-title">
            <div className="store-section-heading">
              <Coins aria-hidden="true"/>
              <div><h2 id="sandbox-chips-title">Fichas sandbox</h2>
                <p>Resgate sua recompensa ou adicione saldo para buy-ins. Fichas não têm saque nem conversão em
                  dinheiro.</p></div>
            </div>

            <div className="store-wallet" role="group" aria-label="Seu saldo sandbox">
              <span className="store-wallet-icon"><Coins aria-hidden="true"/></span>
              <span className="store-wallet-copy"><small>Seu saldo agora</small>
                <strong>{sandboxBalance === undefined ? '—' : `${sandboxBalance.toLocaleString('pt-BR')} fichas`}</strong>
              </span>
              <span className="store-wallet-note">Saldo exclusivo do modo sandbox.</span>
            </div>

            <section className="store-department-reward" aria-labelledby="daily-reward-title"><DailyRewardPanel/></section>

            <div className="store-subsection-heading">
              <h3 id="credit-packs-title">Pacotes de fichas</h3>
              <p>Compare o total, a quantidade base e o bônus. O pagamento é feito por Pix.</p>
            </div>
            <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetryAction={() => void skus.refetch()} onSelectAction={(sku, trigger) => openFrom(trigger, () => void selectSku(sku))}
                     pendingSku={pendingSku}/>
            <section className="store-department-history" aria-labelledby="chip-activity-title">
              <h3 id="chip-activity-title"><Clock3 aria-hidden="true"/> Compras e estornos de fichas</h3>
              <PurchaseHistoryList purchases={purchases.items} isLoading={purchases.isLoading}
                                   isError={purchases.isError} onRetryAction={() => void purchases.refetch()}
                                   onRefund={(purchase, trigger) =>
                                     openFrom(trigger, () => setRefundPurchaseTarget(purchase))}
                                   onResume={(id, trigger) => openFrom(trigger, () => void resume(id))}
                                   resumingId={resumingId}
                                   hasMore={purchases.hasMore} isLoadingMore={purchases.isLoadingMore}
                                   onLoadMoreAction={purchases.loadMore}/>
            </section>
          </section>

          <section id="activity" className="store-section store-department"
                   aria-labelledby="activity-title">
            <div className="store-section-heading">
              <Clock3 aria-hidden="true"/>
              <div><h2 id="activity-title">Compras e estornos</h2>
                <p>Recibos de tudo que você liberou. As compras de fichas ficam na seção Fichas sandbox, junto do
                  botão de estorno.</p></div>
            </div>
            <div className="store-activity-groups">
              <section aria-labelledby="reaction-activity-title">
                <h3 id="reaction-activity-title">Reações</h3>
                <PurchaseActivityList rows={reactionActivityRows(reactionPurchases.items)}
                                      isLoading={reactionPurchases.isLoading} isError={reactionPurchases.isError}
                                      onRetryAction={() => void reactionPurchases.refetch()}
                                      loadingLabel="Carregando compras de reações…"
                                      errorLabel="Não foi possível carregar as compras de reações."
                                      emptyLabel="Suas compras de reações aparecerão aqui."/>
              </section>
              <section aria-labelledby="cosmetic-activity-title">
                <h3 id="cosmetic-activity-title">Baralhos e feltros</h3>
                <PurchaseActivityList
                  rows={cosmeticActivityRows([...deckPurchases.items, ...feltPurchases.items])}
                  isLoading={deckPurchases.isLoading || feltPurchases.isLoading}
                  isError={deckPurchases.isError || feltPurchases.isError}
                  onRetryAction={() => {
                    void deckPurchases.refetch();
                    void feltPurchases.refetch();
                  }}
                  loadingLabel="Carregando compras de cosméticos…"
                  errorLabel="Não foi possível carregar as compras de baralhos e feltros."
                  emptyLabel="Suas compras de baralhos e feltros aparecerão aqui."/>
              </section>
            </div>
          </section>
        </div>
      </AppPageBody>
    </AppPage>
    <PurchaseModal key={activePurchase?.purchase_id ?? 'closed_purchase'} purchase={activePurchase}
                   finalFocusRef={purchaseTriggerRef}
                   onCloseAction={closePurchase} onUpdateAction={setActivePurchase}
                   onRegenerateAction={regeneratePurchase}/>
    <RefundConfirmationDialog key={refundPurchaseTarget?.purchase_id ?? 'closed_refund'} purchase={refundPurchaseTarget}
                              sandboxBalance={player.data?.sandbox_balance} finalFocusRef={purchaseTriggerRef}
                              onCloseAction={() => setRefundPurchaseTarget(null)} onConfirmAction={refund}/>
    <ReactionPurchaseDialog key={`${reactionTarget?.id ?? 'closed'}:${activeReactionPurchase?.purchase_id ?? 'new'}`}
                            entry={reactionTarget} initialPurchase={activeReactionPurchase}
                            sandboxBalance={player.data?.sandbox_balance} finalFocusRef={purchaseTriggerRef}
                            onCloseAction={() => {
                              setReactionTarget(null);
                              setActiveReactionPurchase(undefined);
                            }}/>
    <ReactionRefundDialog key={reactionRefundTarget?.purchase_id ?? 'closed-reaction-refund'}
                          purchase={reactionRefundTarget} finalFocusRef={purchaseTriggerRef}
                          onCloseAction={() => setReactionRefundTarget(null)}
                          onConfirmAction={refundReaction}/>
    <CosmeticPurchaseDialog key={`deck:${deckTarget?.id ?? 'closed'}:${activeDeckPurchase?.purchase_id ?? 'new'}`}
                            kind="deck" entry={deckTarget} initialPurchase={activeDeckPurchase}
                            sandboxBalance={player.data?.sandbox_balance} finalFocusRef={purchaseTriggerRef}
                            onCloseAction={() => {
                              setDeckTarget(null);
                              setActiveDeckPurchase(undefined);
                            }}/>
    <CosmeticRefundDialog key={deckRefundTarget?.purchase_id ?? 'closed-deck-refund'} kind="deck"
                          purchase={deckRefundTarget} finalFocusRef={purchaseTriggerRef}
                          onCloseAction={() => setDeckRefundTarget(null)}
                          onConfirmAction={refundDeck}/>
    <CosmeticPurchaseDialog key={`felt:${feltTarget?.id ?? 'closed'}:${activeFeltPurchase?.purchase_id ?? 'new'}`}
                            kind="felt" entry={feltTarget} initialPurchase={activeFeltPurchase}
                            sandboxBalance={player.data?.sandbox_balance} finalFocusRef={purchaseTriggerRef}
                            onCloseAction={() => {
                              setFeltTarget(null);
                              setActiveFeltPurchase(undefined);
                            }}/>
    <CosmeticRefundDialog key={feltRefundTarget?.purchase_id ?? 'closed-felt-refund'} kind="felt"
                          purchase={feltRefundTarget} finalFocusRef={purchaseTriggerRef}
                          onCloseAction={() => setFeltRefundTarget(null)}
                          onConfirmAction={refundFelt}/>
  </TermsGate>;
}
