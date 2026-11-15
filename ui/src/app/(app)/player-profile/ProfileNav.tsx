'use client';
import {REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';

/** Anchors are the section titles' existing ids, so the nav and `aria-labelledby` can never drift apart. */
const SECTIONS = [
  {id: 'player-profile-identity-title', label: 'Identidade'},
  {id: 'player-profile-avatar-decoration-title', label: 'Avatar'},
  {id: 'player-profile-showcase-title', label: 'Vitrine'},
  {id: 'player-profile-table-title', label: 'Mesa'},
  {id: 'player-profile-chat-filter-title', label: 'Chat'},
  {id: 'player-profile-balances-title', label: 'Saldos'},
  {id: 'player-profile-responsible-title', label: 'Jogo responsável', realMoney: true},
  {id: 'player-profile-wallet-alerts-title', label: 'Alertas'},
] as const;

/** Sticky jump list for a page that is several screens long; scrolls sideways on a phone. */
export function ProfileNav() {
  return <nav className="player-profile-nav" aria-label="Seções do perfil">
    {SECTIONS.filter(section => !('realMoney' in section) || REAL_MONEY_UI_ENABLED).map(section =>
      <a key={section.id} href={`#${section.id}`}>{section.label}</a>)}
  </nav>;
}
