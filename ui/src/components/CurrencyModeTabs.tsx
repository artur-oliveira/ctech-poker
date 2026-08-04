'use client';

import type {WalletMode} from '@/lib/api/player';
import {FilterGroup} from '@/components/FilterGroup';
import {REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';

export function CurrencyModeTabs({mode, onChangeAction}: {
  mode: WalletMode;
  onChangeAction: (mode: WalletMode) => void;
}) {
  const modes = [
    {value: 'sandbox', label: 'Sandbox'},
    {
      value: 'real',
      label: REAL_MONEY_UI_ENABLED ? 'Dinheiro real' : 'Dinheiro real · Indisponível',
      disabled: !REAL_MONEY_UI_ENABLED,
      title: REAL_MONEY_UI_ENABLED ? undefined : 'Este modo ainda não está disponível.'
    }
  ] as const;

  return <FilterGroup label="Modo das estatísticas" value={mode} options={modes} onChangeAction={onChangeAction}/>;
}
