'use client';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {DeckPicker} from '@/components/profile/DeckPicker';
import {type PlayerProfile, type WalletMode} from '@/lib/api/player';
import {availableWalletMode, REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';
import {useTablePreferenceSave} from '@/lib/hooks/useProfileEdits';

/**
 * Cosmetics and mode: what the next hand is dealt with. The deck picker is the
 * same component the header popover renders (loaded on demand there, because
 * that menu is on every authenticated page); the wallet-mode switch lives only
 * here, where there is room to say what it does.
 */
export function TableSection({me}: {me: PlayerProfile}) {
  const save = useTablePreferenceSave();

  // Coerce to chips whenever the real-money UI is gated off, so nothing here
  // implies real money is active.
  const walletMode: WalletMode = availableWalletMode(me.wallet_mode);

  return <section className="player-profile-section" aria-labelledby="player-profile-table-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-table-title">Sua mesa</h2>
      <p>Preferências aplicadas na próxima mão.</p>
    </header>

    {REAL_MONEY_UI_ENABLED && <div className="player-profile-setting">
      <span><Label id="wallet-mode-label">{walletMode === 'real' ? 'Dinheiro real' : 'Fichas'}</Label>
        <small>Modo de jogo</small></span>
      <Switch aria-labelledby="wallet-mode-label" checked={walletMode === 'real'}
              onCheckedChange={checked => save.mutate({wallet_mode: checked ? 'real' : 'sandbox'})}/>
    </div>}

    <DeckPicker className="player-profile-deck" deckVariant={me.deck_variant}/>
  </section>;
}
