'use client';
import type {ReactNode} from 'react';
import Image from 'next/image';
import {LockKeyhole, QrCode, RotateCcw, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import type {CosmeticCatalogEntry, CosmeticKind, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';
import type {ActivityRow} from '@/components/store/PurchaseActivityList';
import {currentCosmeticPurchase} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

const STATUS_LABEL: Record<string, string> = {
  processing: 'Processando', pending: 'Aguardando Pix', confirmed: 'Liberada', refunding: 'Estornando',
  refunded: 'Estornada', expired: 'Expirada', failed: 'Falhou'
};

function formatBRL(cents?: number) {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

interface CosmeticSectionProps {
  catalog: CosmeticCatalogEntry[];
  purchases: CosmeticPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  // Each action receives the button that triggered it: the dialogs are opened
  // programmatically, so base-ui has nothing to restore focus to on close
  // unless we hand it the trigger ourselves.
  onBuyAction: (entry: CosmeticCatalogEntry, trigger: HTMLButtonElement) => void;
  onRefundAction: (purchase: CosmeticPurchase, trigger: HTMLButtonElement) => void;
  onResumeAction: (entry: CosmeticCatalogEntry, purchase: CosmeticPurchase, trigger: HTMLButtonElement) => void;
}

function CosmeticGrid({kind, labelFor, renderPreview, ariaLabel, loadingLabel, emptyLabel, catalog, purchases,
                       isLoading, isError, onRetryAction, onBuyAction, onRefundAction, onResumeAction}: CosmeticSectionProps & {
  kind: CosmeticKind;
  labelFor: (id: string) => string | undefined;
  renderPreview: (id: string) => ReactNode;
  ariaLabel: string;
  loadingLabel: string;
  emptyLabel: string;
}) {
  if (isLoading) return <SkeletonList label={loadingLabel} count={4} height={96} className="cosmetic-store-grid"/>;
  if (isError) return <div className="lobby-empty">Não foi possível carregar o catálogo agora.
    <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button></div>;

  // Always-visible gallery: every catalog entry for this kind, owned and unowned, free and premium.
  const entries = catalog.filter(entry => entry.kind === kind && labelFor(entry.id));
  if (!entries.length) return <div className="lobby-empty"><Sparkles aria-hidden="true"/><p>{emptyLabel}</p></div>;

  return <ul className="cosmetic-store-grid" aria-label={ariaLabel}>
    {entries.map(entry => {
      const purchase = entry.premium ? currentCosmeticPurchase(purchases, entry.id) : undefined;
      // Ownership is the catalog's server-side entitlement flag (always true for
      // free items); the purchase row is only what a refund needs in order to
      // name a receipt. The two can disagree — history is paginated, and a
      // refunded item leaves rows behind — and ownership wins when they do.
      const owned = entry.owned;
      const active = purchase?.status === 'pending' || purchase?.status === 'processing';
      const refunding = purchase?.status === 'refunding';
      return <li key={entry.id} className={`cosmetic-store-item${owned ? ' owned' : ''}`}>
        <span className="cosmetic-store-preview" aria-hidden="true">{renderPreview(entry.id)}</span>
        <span className="cosmetic-store-copy"><strong>{labelFor(entry.id)}</strong>
          {entry.premium && <small>{formatBRL(entry.price_cents)} <span aria-hidden="true">·</span>
            {(entry.price_fichas ?? 0).toLocaleString('pt-BR')} fichas</small>}</span>
        {!entry.premium ? <span className="cosmetic-store-free">Grátis</span>
          : owned ? <span className="cosmetic-store-owned"><Sparkles aria-hidden="true"/> Sua</span>
            : active || refunding ? <span className="cosmetic-store-state">{STATUS_LABEL[purchase!.status]}</span>
              : <span className="cosmetic-store-lock"><LockKeyhole aria-hidden="true"/> Não liberada</span>}
        <span className="cosmetic-store-action">
          {!entry.premium ? null
            : owned ? (purchase && <Button type="button" variant="ghost" size="sm"
                                           onClick={event => onRefundAction(purchase, event.currentTarget)}>
                <RotateCcw aria-hidden="true"/> Estornar</Button>)
              : refunding ? <span className="cosmetic-store-wait">Aguarde</span>
                : active ? <Button type="button" variant="outline" size="sm"
                                   onClick={event => onResumeAction(entry, purchase!, event.currentTarget)}>
                  <QrCode aria-hidden="true"/> Acompanhar</Button>
                : <Button type="button" size="sm" onClick={event => onBuyAction(entry, event.currentTarget)}>Liberar</Button>}
        </span>
      </li>;
    })}
  </ul>;
}

export function DeckStoreSection(props: CosmeticSectionProps) {
  return <CosmeticGrid {...props} kind="deck" labelFor={id => DECK_VARIANTS[id as DeckVariantId]?.label}
    renderPreview={id => <span className="cosmetic-store-deck-preview">
      {ACES.map(card => <Image key={card} src={cardPath(card, id as DeckVariantId)} alt="" width={14} height={20}/>)}
    </span>}
    ariaLabel="Catálogo de baralhos" loadingLabel="Carregando baralhos…"
    emptyLabel="Nenhum baralho disponível no momento."/>;
}

export function FeltStoreSection(props: CosmeticSectionProps) {
  return <CosmeticGrid {...props} kind="felt" labelFor={id => TABLE_THEMES[id as TableThemeId]?.label}
    renderPreview={id => {
      const theme = TABLE_THEMES[id as TableThemeId];
      return <span className="felt-swatch"
                   style={{'--theme-a': theme?.colors[0], '--theme-b': theme?.colors[1]} as React.CSSProperties}/>;
    }}
    ariaLabel="Catálogo de feltros" loadingLabel="Carregando feltros…"
    emptyLabel="Nenhum feltro disponível no momento."/>;
}

// Maps deck/felt purchases into the shared activity-list row shape. Deck and
// felt ids never collide, so one mapper covers both kinds.
export function cosmeticActivityRows(purchases: CosmeticPurchase[]): ActivityRow[] {
  return purchases.map(item => {
    const label = item.kind === 'deck'
      ? DECK_VARIANTS[item.item_id as DeckVariantId]?.label
      : TABLE_THEMES[item.item_id as TableThemeId]?.label;
    const date = formatDate(item.updated_at || item.created_at);
    const kindLabel = item.kind === 'deck' ? 'Baralho' : 'Feltro';
    return {
      id: item.purchase_id,
      label: label || item.item_id,
      detail: `${kindLabel} · ${item.method === 'pix' ? 'Pix' : 'Fichas'}${date ? ` · ${date}` : ''}`,
      status: item.status,
      statusLabel: STATUS_LABEL[item.status] || 'Atualizando',
      media: item.kind === 'deck'
        ? <Image src={cardPath('As', item.item_id as DeckVariantId)} alt="" width={18} height={25}/>
        : <span className="felt-swatch" style={{
          '--theme-a': TABLE_THEMES[item.item_id as TableThemeId]?.colors[0],
          '--theme-b': TABLE_THEMES[item.item_id as TableThemeId]?.colors[1],
        } as React.CSSProperties}/>,
      at: item.updated_at || item.created_at || '',
    };
  });
}

function formatDate(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('pt-BR', {day: '2-digit', month: 'short'});
}
