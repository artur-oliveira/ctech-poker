'use client';
import {LockKeyhole, QrCode, RotateCcw, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {EmojiGlyph} from '@/components/ui/EmojiGlyph';
import {SkeletonList} from '@/components/ui/skeleton';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
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
  onBuyAction: (entry: ReactionCatalogEntry) => void;
  onRefundAction: (purchase: ReactionPurchase) => void;
  onResumeAction: (entry: ReactionCatalogEntry, purchase: ReactionPurchase) => void;
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
        const owned = purchase?.status === 'confirmed';
        const active = purchase?.status === 'pending' || purchase?.status === 'processing';
        const refunding = purchase?.status === 'refunding';
        return <li key={entry.id} className={`reaction-store-item${owned ? ' owned' : ''}`}>
          <span className="reaction-store-glyph"><EmojiGlyph glyph={definition.glyph}/></span>
          <span className="reaction-store-copy"><strong>{definition.label}</strong>
            <small>{formatBRL(entry.price_cents)} <span aria-hidden="true">·</span> {(entry.price_fichas ?? 0).toLocaleString('pt-BR')} fichas</small></span>
          {owned ? <span className="reaction-store-owned"><Sparkles aria-hidden="true"/> Sua</span>
            : active || refunding ? <span className="reaction-store-state">{STATUS_LABEL[purchase.status]}</span>
              : <span className="reaction-store-lock"><LockKeyhole aria-hidden="true"/> Bloqueada</span>}
          <span className="reaction-store-action">
            {owned ? <Button type="button" variant="ghost" size="sm" onClick={() => onRefundAction(purchase)}>
                <RotateCcw aria-hidden="true"/> Estornar</Button>
              : refunding ? <span className="reaction-store-wait">Aguarde</span>
                : active ? <Button type="button" variant="outline" size="sm" onClick={() => onResumeAction(entry, purchase)}>
                  <QrCode aria-hidden="true"/> Acompanhar</Button>
                : <Button type="button" size="sm" onClick={() => onBuyAction(entry)}>Liberar</Button>}
          </span>
        </li>;
      })}
    </ul>;
}

export function ReactionPurchaseHistory({purchases, isLoading = false, isError = false, onRetryAction}: {
  purchases: ReactionPurchase[];
  isLoading?: boolean;
  isError?: boolean;
  onRetryAction?: () => void;
}) {
  if (isLoading) return <SkeletonList label="Carregando compras de reações…" count={2} height={58}
    className="reaction-history-list"/>;
  if (isError) return <div className="store-activity-error">Não foi possível carregar as compras de reações.
    {onRetryAction && <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button>}
  </div>;

  const history = [...purchases].sort((a, b) =>
    (b.updated_at || b.created_at || '').localeCompare(a.updated_at || a.created_at || '')).slice(0, 4);

  if (!history.length) return <p className="store-history-empty">Suas compras de reações aparecerão aqui.</p>;

  return <ul className="reaction-history-list">{history.map(item => {
        const definition = TABLE_REACTIONS[item.reaction_id as TableReactionID];
        return <li key={item.purchase_id}><span>{definition?.glyph && <EmojiGlyph glyph={definition.glyph}/>}</span>
          <span><strong>{definition?.label || item.reaction_id}</strong><small>{item.method === 'pix' ? 'Pix' : 'Fichas'}{formatDate(item.updated_at || item.created_at) ? ` · ${formatDate(item.updated_at || item.created_at)}` : ''}</small></span>
          <b className={`reaction-history-status ${item.status}`}>{STATUS_LABEL[item.status] || 'Atualizando'}</b>
        </li>;
      })}</ul>;
}
