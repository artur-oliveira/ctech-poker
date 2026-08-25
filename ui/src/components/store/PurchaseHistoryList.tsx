'use client';
import {useState} from 'react';
import {ChevronDown, ChevronUp, QrCode, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import type {SandboxPurchase} from '@/lib/api/wallet';

const STATUS_LABEL: Record<string, string> = {
  pending: 'Pendente',
  confirmed: 'Confirmada',
  refunded: 'Estornada',
  expired: 'Expirada',
  failed: 'Falhou',
};

function formatBRL(cents?: number) {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

function formatDate(iso?: string) {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return 'Data indisponível';
  return date.toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit'
  });
}

export function PurchaseHistoryList({purchases, isLoading, isError, onRetryAction, onRefund,
                                     onResume, resumingId, hasMore = false, isLoadingMore = false,
                                     onLoadMoreAction}: {
  purchases: SandboxPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  onRefund: (purchase: SandboxPurchase, trigger: HTMLButtonElement) => void;
  onResume: (purchaseId: string, trigger: HTMLButtonElement) => void;
  resumingId: string | null;
  // History is cursor-paginated: `purchases` is everything fetched so far, and
  // hasMore says the server still has older rows behind the next cursor.
  hasMore?: boolean;
  isLoadingMore?: boolean;
  onLoadMoreAction?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);

  if (isLoading) {
    return <SkeletonList label="Carregando histórico de compras…" count={3} height={64} className="store-history"/>;
  }
  if (isError) {
    return <div className="lobby-empty">Não foi possível carregar seu histórico agora.
      <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button>
    </div>;
  }
  if (purchases.length === 0) {
    return <p className="store-history-empty">Suas compras via Pix aparecerão aqui.</p>;
  }

  const visiblePurchases = expanded ? purchases : purchases.slice(0, 3);

  return <>
    <ul id="store-history-list" className="store-history">
    {visiblePurchases.map(p => <li key={p.purchase_id} className="store-history-item">
      <div className="store-history-info">
        <strong className="store-history-credits">{(p.total_credits ?? 0).toLocaleString('pt-BR')} fichas</strong>
        <span className="store-history-value">{formatBRL(p.price_cents)}</span>
        <time className="store-history-date" dateTime={p.created_at}>{formatDate(p.created_at)}</time>
      </div>
      <span className={`store-status ${STATUS_LABEL[p.status] ? p.status : 'unknown'}`}>
        {STATUS_LABEL[p.status] || 'Desconhecida'}
      </span>
      {p.status === 'pending' && <Button type="button" variant="outline" size="sm"
                                          disabled={resumingId === p.purchase_id}
                                          onClick={event => onResume(p.purchase_id, event.currentTarget)}>
        <QrCode/> {resumingId === p.purchase_id ? 'Abrindo…' : 'Continuar pagamento'}
      </Button>}
      {p.status === 'confirmed' && <Button type="button" variant="outline" size="sm"
                                            onClick={event => onRefund(p, event.currentTarget)}>
        <RotateCcw/> Solicitar estorno
      </Button>}
    </li>)}
    </ul>
    {(purchases.length > 3 || hasMore) && <Button type="button" variant="ghost" size="sm"
      className="store-history-toggle" aria-expanded={expanded} aria-controls="store-history-list"
      onClick={() => setExpanded(value => !value)}>
      {expanded ? <ChevronUp aria-hidden="true"/> : <ChevronDown aria-hidden="true"/>}
      {expanded ? 'Mostrar menos' : `Ver todas as ${purchases.length}${hasMore ? '+' : ''} compras`}
    </Button>}
    {expanded && hasMore && onLoadMoreAction && <Button type="button" variant="outline" size="sm"
      className="store-history-more" disabled={isLoadingMore} onClick={onLoadMoreAction}>
      {isLoadingMore ? 'Carregando…' : 'Carregar mais compras'}
    </Button>}
  </>;
}
