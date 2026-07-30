'use client';
import {RotateCcw} from 'lucide-react';
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
  return new Date(iso).toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit'
  });
}

export function PurchaseHistoryList({purchases, isLoading, isError, onRetry, onRefund, refundingId}: {
  purchases: SandboxPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onRefund: (purchaseId: string) => void;
  refundingId: string | null;
}) {
  if (isLoading) {
    return <SkeletonList label="Carregando histórico de compras…" count={3} height={64} className="store-history"/>;
  }
  if (isError) {
    return <div className="lobby-empty">Não foi possível carregar seu histórico agora.
      <Button variant="outline" size="sm" onClick={onRetry}>Tentar novamente</Button>
    </div>;
  }
  if (purchases.length === 0) {
    return <p className="store-history-empty">Você ainda não comprou créditos.</p>;
  }

  return <ul className="store-history">
    {purchases.map(p => <li key={p.purchase_id} className="store-history-item">
      <div className="store-history-info">
        <strong>{(p.total_credits ?? 0).toLocaleString('pt-BR')} fichas · {formatBRL(p.price_cents)}</strong>
        <small>{formatDate(p.created_at)}</small>
      </div>
      <span className={`store-status ${p.status}`}>{STATUS_LABEL[p.status] || p.status}</span>
      {p.status === 'confirmed' && <Button type="button" variant="outline" size="sm"
                                            disabled={refundingId === p.purchase_id}
                                            onClick={() => onRefund(p.purchase_id)}>
        <RotateCcw/> {refundingId === p.purchase_id ? 'Estornando…' : 'Estornar'}
      </Button>}
    </li>)}
  </ul>;
}
