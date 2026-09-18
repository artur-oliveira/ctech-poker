'use client';

import type {WalletMode} from '@/lib/api/player';
import {FilterGroup} from '@/components/FilterGroup';
import {REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';

export function CurrencyModeTabs({mode, onChangeAction, showLabel}: {
  mode: WalletMode;
  onChangeAction: (mode: WalletMode) => void;
  showLabel?: boolean;
}) {
  const modes = [
    {value: 'sandbox', label: 'Fichas'},
    {
      value: 'real',
      label: REAL_MONEY_UI_ENABLED ? 'Dinheiro real' : 'Dinheiro real · Indisponível',
      disabled: !REAL_MONEY_UI_ENABLED,
      title: REAL_MONEY_UI_ENABLED ? undefined : 'Ainda não disponível.'
    }
  ] as const;

  return <FilterGroup label="Carteira" value={mode} options={modes} onChangeAction={onChangeAction} showLabel={showLabel}/>;
}
