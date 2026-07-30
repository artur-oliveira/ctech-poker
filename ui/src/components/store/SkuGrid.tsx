'use client';
import {Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import type {SandboxSKU} from '@/lib/api/wallet';

function formatBRL(cents: number) {
  return (cents / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

export function SkuGrid({skus, isLoading, isError, onRetry, onSelect, pendingSku}: {
  skus: SandboxSKU[];
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onSelect: (sku: SandboxSKU) => void;
  pendingSku: string | null;
}) {
  if (isLoading) {
    return <SkeletonList label="Carregando pacotes de créditos…" count={4} height={140} className="store-sku-grid"/>;
  }
  if (isError) {
    return <div className="lobby-empty">Não foi possível carregar os pacotes agora.
      <Button variant="outline" size="sm" onClick={onRetry}>Tentar novamente</Button>
    </div>;
  }
  if (skus.length === 0) {
    return <div className="lobby-empty">
      <Sparkles aria-hidden="true"/>
      <p>Nenhum pacote disponível no momento.</p>
    </div>;
  }

  return <div className="store-sku-grid">
    {skus.map(sku => <button key={sku.id} type="button" className="store-sku-card"
                              disabled={pendingSku !== null} onClick={() => onSelect(sku)}>
      {sku.bonus_percent > 0 && <span className="store-sku-bonus">+{sku.bonus_percent}% bônus</span>}
      <span className="store-sku-credits">{sku.total_credits.toLocaleString('pt-BR')} <small>fichas</small></span>
      <span className="store-sku-price">{pendingSku === sku.id ? 'Abrindo…' : formatBRL(sku.price_cents)}</span>
    </button>)}
  </div>;
}
