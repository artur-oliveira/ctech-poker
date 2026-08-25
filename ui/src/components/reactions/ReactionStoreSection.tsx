'use client';
import {LockKeyhole, QrCode, RotateCcw, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {EmojiGlyph} from '@/components/ui/EmojiGlyph';
import {SkeletonList} from '@/components/ui/skeleton';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import {PurchaseActivityList, type ActivityRow} from '@/components/store/PurchaseActivityList';
import {currentReactionPurchase} from '@/lib/api/reactionPurchases';
import {TABLE_REACTIONS, type TableReactionID} from '@/lib/reactions';

const STATUS_LABEL: Record<string, string> = {
  processing: 'Processando', pending: 'Aguardando Pix', confirmed: 'Liberada', refunding: 'Estornando',
  refunded: 'Estornada', expired: 'Expirada', failed: 'Falhou'
};

function formatBRL(cents?: number) {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

function formatDate(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('pt-BR', {day: '2-digit', month: 'short'});
}

export function ReactionStoreSection({catalog, purchases, isLoading, isError, onRetryAction, onBuyAction,
                                      onRefundAction, onResumeAction}: {
  catalog: ReactionCatalogEntry[];
  purchases: ReactionPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  // Each action receives the button that triggered it: the dialogs are opened
  // programmatically, so base-ui has nothing to restore focus to on close
  // unless we hand it the trigger ourselves.
  onBuyAction: (entry: ReactionCatalogEntry, trigger: HTMLButtonElement) => void;
  onRefundAction: (purchase: ReactionPurchase, trigger: HTMLButtonElement) => void;
  onResumeAction: (entry: ReactionCatalogEntry, purchase: ReactionPurchase, trigger: HTMLButtonElement) => void;
}) {
  if (isLoading) return <SkeletonList label="Carregando reações premium…" count={3} height={96}
    className="reaction-store-grid"/>;
  if (isError) return <div className="lobby-empty">Não foi possível carregar as reações premium agora.
    <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button></div>;

  const premium = catalog.filter(entry => entry.premium && TABLE_REACTIONS[entry.id as TableReactionID]);
  if (!premium.length) return <div className="lobby-empty"><Sparkles aria-hidden="true"/>
    <p>Nenhuma reação premium disponível no momento.</p></div>;

  return <ul className="reaction-store-grid" aria-label="Catálogo de reações premium">
      {premium.map(entry => {
        const definition = TABLE_REACTIONS[entry.id as TableReactionID];
        const purchase = currentReactionPurchase(purchases, entry.id);
        // Ownership is the catalog's server-side entitlement flag; the purchase
        // row is only what a refund needs in order to name a receipt. The two
        // can disagree (history is paginated, and a refunded item leaves rows
        // behind) — when they do, ownership wins.
        const owned = entry.owned;
        const active = purchase?.status === 'pending' || purchase?.status === 'processing';
        const refunding = purchase?.status === 'refunding';
        return <li key={entry.id} className={`reaction-store-item${owned ? ' owned' : ''}`}>
          <span className="reaction-store-glyph"><EmojiGlyph glyph={definition.glyph}/></span>
          <span className="reaction-store-copy"><strong>{definition.label}</strong>
            <small>{formatBRL(entry.price_cents)} <span aria-hidden="true">·</span> {(entry.price_fichas ?? 0).toLocaleString('pt-BR')} fichas</small></span>
          {owned ? <span className="reaction-store-owned"><Sparkles aria-hidden="true"/> Sua</span>
            : active || refunding ? <span className="reaction-store-state">{STATUS_LABEL[purchase.status]}</span>
              : <span className="reaction-store-lock"><LockKeyhole aria-hidden="true"/> Não liberada</span>}
          <span className="reaction-store-action">
            {owned ? (purchase && <Button type="button" variant="ghost" size="sm" onClick={event => onRefundAction(purchase, event.currentTarget)}>
                <RotateCcw aria-hidden="true"/> Estornar</Button>)
              : refunding ? <span className="reaction-store-wait">Aguarde</span>
                : active ? <Button type="button" variant="outline" size="sm" onClick={event => onResumeAction(entry, purchase, event.currentTarget)}>
                  <QrCode aria-hidden="true"/> Acompanhar</Button>
                : <Button type="button" size="sm" onClick={event => onBuyAction(entry, event.currentTarget)}>Liberar</Button>}
          </span>
        </li>;
      })}
    </ul>;
}

// Maps reaction purchases into the shared activity-list row shape.
export function reactionActivityRows(purchases: ReactionPurchase[]): ActivityRow[] {
  return purchases.map(item => {
    const definition = TABLE_REACTIONS[item.reaction_id as TableReactionID];
    const date = formatDate(item.updated_at || item.created_at);
    return {
      id: item.purchase_id,
      label: definition?.label || item.reaction_id,
      detail: `${item.method === 'pix' ? 'Pix' : 'Fichas'}${date ? ` · ${date}` : ''}`,
      status: item.status,
      statusLabel: STATUS_LABEL[item.status] || 'Atualizando',
      media: definition?.glyph ? <EmojiGlyph glyph={definition.glyph}/> : undefined,
      at: item.updated_at || item.created_at || '',
    };
  });
}

/** Backwards-compatible composition used by focused tests and any caller that
 * still wants the reaction-only history block. The store route composes the
 * same rows into its shared purchase activity list. */
export function ReactionPurchaseHistory({purchases, isLoading = false, isError = false, onRetryAction}: {
  purchases: ReactionPurchase[];
  isLoading?: boolean;
  isError?: boolean;
  onRetryAction?: () => void;
}) {
  return <PurchaseActivityList rows={reactionActivityRows(purchases)} isLoading={isLoading} isError={isError}
    loadingLabel="Carregando compras de reações…"
    errorLabel="Não foi possível carregar suas compras de reações agora."
    emptyLabel="Suas compras de reações aparecerão aqui."
    onRetryAction={onRetryAction}/>;
}
