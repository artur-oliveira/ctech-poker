'use client';
import {DeckPicker} from '@/components/profile/DeckPicker';
import {WalletModeSetting} from '@/components/profile/WalletModeSetting';
import {type PlayerProfile} from '@/lib/api/player';

/**
 * Cosmetics and mode: what the next hand is dealt with. The deck picker is the
 * same component the header popover renders (loaded on demand there, because
 * that menu is on every authenticated page); the wallet-mode control sits next to it
 * in both places.
 */
export function TableSection({me}: {me: PlayerProfile}) {
  return <section className="player-profile-section" aria-labelledby="player-profile-table-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-table-title">Sua mesa</h2>
      <p>Preferências aplicadas na próxima mão.</p>
    </header>

    <WalletModeSetting walletMode={me.wallet_mode}/>

    <DeckPicker className="player-profile-deck" deckVariant={me.deck_variant}/>
  </section>;
}
