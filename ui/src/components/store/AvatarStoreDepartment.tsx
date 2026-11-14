'use client';
import type {ComponentType, RefObject} from 'react';
import {useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import type {LucideIcon} from 'lucide-react';
import {CosmeticPurchaseDialog} from '@/components/store/CosmeticPurchaseDialog';
import {CosmeticRefundDialog} from '@/components/store/CosmeticRefundDialog';
import type {CosmeticSectionProps} from '@/components/store/CosmeticStoreSection';
import {
  type AvatarStoreKind, type CosmeticCatalogEntry, type CosmeticPurchase, getCosmeticPurchase, refundCosmeticPurchase
} from '@/lib/api/cosmeticPurchases';
import {WALLET_QUERY_ROOT} from '@/lib/api/wallet';

/**
 * One avatar department of the store (frame, hat or accessory): the section,
 * its grid and its own purchase/refund dialogs, the same lifecycle the deck
 * and felt departments run inline in the store page. The page owns the
 * queries (catalog and history are both armed by this section coming into
 * view) and hands the grid one `isLoading` that stays true until then, so the
 * department is a skeleton or a whole grid, never a half-painted one.
 */
export function AvatarStoreDepartment({kind, anchor, title, description, icon: Icon, Grid, sectionRef, catalog,
                                       purchases, isLoading, isError, onRetryAction, sandboxBalance, finalFocusRef,
                                       openFrom}: {
  kind: AvatarStoreKind;
  anchor: string;
  title: string;
  description: string;
  icon: LucideIcon;
  Grid: ComponentType<CosmeticSectionProps>;
  sectionRef: (node: Element | null) => void;
  catalog: CosmeticCatalogEntry[];
  purchases: CosmeticPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  sandboxBalance?: number;
  finalFocusRef: RefObject<HTMLButtonElement | null>;
  openFrom: (trigger: HTMLButtonElement, open: () => unknown) => unknown;
}) {
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<CosmeticCatalogEntry | null>(null);
  const [active, setActive] = useState<CosmeticPurchase | undefined>();
  const [refundTarget, setRefundTarget] = useState<CosmeticPurchase | null>(null);

  async function refund(purchaseId: string) {
    await refundCosmeticPurchase(kind, purchaseId);
    await Promise.all([
      queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT}),
      queryClient.invalidateQueries({queryKey: ['player', 'me']}),
    ]);
  }

  return <>
    <section id={anchor} ref={sectionRef} className="store-section store-department cosmetic-store-section"
             aria-labelledby={`${anchor}-title`}>
      <div className="store-section-heading">
        <Icon aria-hidden="true"/>
        <div><h2 id={`${anchor}-title`}>{title}</h2><p>{description}</p></div>
      </div>
      <Grid catalog={catalog} purchases={purchases} isLoading={isLoading} isError={isError}
            onRetryAction={onRetryAction}
            onBuyAction={(entry, trigger) => openFrom(trigger, () => {
              setActive(undefined);
              setTarget(entry);
            })}
            onResumeAction={(entry, purchase, trigger) => openFrom(trigger, async () => {
              setTarget(entry);
              try {
                setActive(await getCosmeticPurchase(kind, purchase.purchase_id));
              } catch {
                setActive(purchase);
              }
            })}
            onRefundAction={(purchase, trigger) => openFrom(trigger, () => setRefundTarget(purchase))}/>
    </section>
    <CosmeticPurchaseDialog key={`${kind}:${target?.id ?? 'closed'}:${active?.purchase_id ?? 'new'}`}
                            kind={kind} entry={target} initialPurchase={active} sandboxBalance={sandboxBalance}
                            finalFocusRef={finalFocusRef}
                            onCloseAction={() => {
                              setTarget(null);
                              setActive(undefined);
                            }}/>
    <CosmeticRefundDialog key={refundTarget?.purchase_id ?? `closed-${kind}-refund`} kind={kind}
                          purchase={refundTarget} finalFocusRef={finalFocusRef}
                          onCloseAction={() => setRefundTarget(null)} onConfirmAction={refund}/>
  </>;
}
