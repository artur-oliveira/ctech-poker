'use client';

import type {WalletMode} from '@/lib/api/player';

export function CurrencyModeTabs({mode, onChange}: {
  mode: WalletMode;
  onChange: (mode: WalletMode) => void;
}) {
  return <div className="filter-tabs" role="tablist" aria-label="Modo das estatísticas">
    <button type="button" role="tab" aria-selected={mode === 'sandbox'}
            className={`filter-tab${mode === 'sandbox' ? ' active' : ''}`}
            onClick={() => onChange('sandbox')}>Sandbox
    </button>
    <button type="button" role="tab" aria-selected={mode === 'real'}
            className={`filter-tab${mode === 'real' ? ' active' : ''}`}
            onClick={() => onChange('real')}>Dinheiro real
    </button>
  </div>;
}
