'use client';
import {Label} from '@aoctech/ui';
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import type {PlayerProfile} from '@/lib/api/player';
import {availableWalletMode, REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';
import {useTablePreferenceSave} from '@/lib/hooks/useProfileEdits';

/**
 * Which wallet the next table is played with. A two-way choice between named
 * modes, so it is a segmented control (both options always visible) rather than
 * a switch whose label flips with its state. Shared by the header popover and
 * the profile page; renders nothing while the real-money UI is gated off.
 */
export function WalletModeSetting({walletMode, className}: {
  walletMode: PlayerProfile['wallet_mode'] | undefined;
  className?: string;
}) {
  const save = useTablePreferenceSave();
  if (!REAL_MONEY_UI_ENABLED) return null;
  return <div className={`wallet-mode-setting${className ? ` ${className}` : ''}`}>
    <Label>Modo de jogo</Label>
    <CurrencyModeTabs mode={availableWalletMode(walletMode)}
                      onChangeAction={mode => {
                        if (mode !== availableWalletMode(walletMode)) save.mutate({wallet_mode: mode});
                      }}/>
  </div>;
}
