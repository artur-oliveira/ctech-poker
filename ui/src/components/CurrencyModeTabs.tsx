'use client';

import type {WalletMode} from '@/lib/api/player';
import {FilterGroup} from '@/components/FilterGroup';

const MODES = [
  {value: 'sandbox', label: 'Sandbox'},
  {value: 'real', label: 'Dinheiro real'}
] as const;

export function CurrencyModeTabs({mode, onChange}: {
  mode: WalletMode;
  onChange: (mode: WalletMode) => void;
}) {
  return <FilterGroup label="Modo das estatísticas" value={mode} options={MODES} onChange={onChange}/>;
}
