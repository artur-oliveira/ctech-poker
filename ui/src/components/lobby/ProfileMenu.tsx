'use client';
import {useState} from 'react';
import dynamic from 'next/dynamic';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {
  Activity,
  Check,
  ChevronRight,
  LoaderCircle,
  LogOut,
  Pencil,
  ShoppingBag,
  Sparkles,
  UserRound,
  WalletCards,
  X
} from 'lucide-react';
import {getMe, type WalletMode} from '@/lib/api/player';
import {endSession, logout} from '@/lib/auth/oauth';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {Skeleton} from '@/components/ui/skeleton';
import {ProfilePhotoEditor, ProfilePhotoRemoveButton} from '@/components/profile/ProfilePhoto';
import {SelfHudDialog} from '@/components/lobby/SelfHudDialog';
import {chipsExact, moneyExact} from '@/lib/chips';
import {PLAYER_ME_KEY, useProfileNameSave} from '@/lib/hooks/useProfileEdits';
import {pushNotification} from '@/lib/notify';
import {availableWalletMode, REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';

/** The deck picker brings the `Select` primitive and the whole variant
 * catalogue with it. This menu is part of `AppPageChrome`, so whatever it
 * imports statically is paid by `/lobby`, `/store`, `/profile`, `/hands`,
 * `/people`, `/achievements`, `/leaderboard` and `/player-profile` alike:
 * importing the picker directly costs those routes ~57 kB of first-load JS
 * each (measured). Loading it on the first open keeps that weight on the
 * players who actually open the menu, and the catalog read travels with it for
 * the same reason (#232). */
const DeckPicker = dynamic(() => import('@/components/profile/DeckPicker').then(module => module.DeckPicker), {
  ssr: false,
  // Reserves the row's height so nothing below it jumps when the chunk lands.
  // Silent on purpose: the popover is opened many times a session and a
  // sub-second chunk fetch is not worth an announcement.
  loading: () => <div className="profile-deck-setting profile-deck-loading" aria-hidden="true">
    <span>Baralho</span><Skeleton style={{height: '44px'}}/>
  </div>
});

/** `logout()` revokes the refresh token under a 3 s deadline and only then
 * redirects through the IdP. Past that the redirect itself is what stalled, so
 * the toast offers the direct end-session exit. Deliberately never cleared: a
 * successful logout navigates away and takes the timer with it. */
const LOGOUT_STALL_MS = 4_000;

function formatChips(amount?: number) {
  return `${chipsExact(amount ?? 0)} fichas`;
}

/**
 * The header shortcut: who you are, what you hold, and the quick edits that
 * fit in 360px — display name, photo and deck. It is not the whole editor.
 * `/player-profile` is the superset (it adds the showcase, the wallet mode and
 * room to explain each field), and this menu links to it.
 *
 * Two surfaces, one implementation: the name write is `useProfileNameSave`,
 * the photo is `ProfilePhotoEditor`, the deck is `DeckPicker`, and the route
 * renders the same three. Nothing here mirrors the profile locally, so a save
 * on either surface repaints the other from `['player','me']`.
 */
export function ProfileMenu() {
  const {data: me} = useQuery({queryKey: PLAYER_ME_KEY, queryFn: getMe});
  // One-way latch: the deck picker's chunk and its catalog read are both
  // deferred until the menu is actually opened, then stay warm for every
  // later open.
  const [menuOpened, setMenuOpened] = useState(false);
  const [name, setName] = useState('');
  const [editingName, setEditingName] = useState(false);
  const [selfHudOpen, setSelfHudOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);
  const nameSave = useProfileNameSave();

  function signOut() {
    if (loggingOut) return;
    setLoggingOut(true);
    window.setTimeout(() => pushNotification(
      'A saída está demorando mais que o normal.', 'error',
      [{label: 'Sair agora', run: () => endSession()}]
    ), LOGOUT_STALL_MS);
    void logout();
  }

  function commitName() {
    if (nameSave.saveName(name)) setEditingName(false);
  }

  // Coerce to chips whenever the real-money UI is gated off, so the pill and
  // the balance label never imply real money is active.
  const walletMode: WalletMode = availableWalletMode(me?.wallet_mode);
  const balanceLabel = walletMode === 'real' ? moneyExact(me?.game_balance) : formatChips(me?.sandbox_balance);

  return <><Popover onOpenChange={(open, details) => {
    if (open) setMenuOpened(true);
    // Escape belongs to the name editor while it is open: cancel the edit and
    // keep the menu where it was, rather than dismissing both at once.
    if (!open && editingName && details.reason === 'escape-key') {
      details.cancel();
      setEditingName(false);
    }
  }}>
    <div className="profile-summary">
      <Link href="/store" className="balance-pill" aria-label={`Abrir loja. Saldo: ${balanceLabel}`}>
        {balanceLabel}
      </Link>
      <PopoverTrigger render={<Button variant="ghost" size="icon" className="rounded-full" aria-label="Abrir perfil"/>}>
        <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url}/>
      </PopoverTrigger>
    </div>
    <PopoverContent className="profile-menu-content" aria-label="Perfil e preferências">
      <div className="profile-menu">
        <header className="profile-menu-identity">
          <ProfilePhotoEditor className="profile-menu-avatar" name={me?.name} avatarUrl={me?.avatar_url} size={64}/>
          <div className="profile-menu-identity-copy">
            <small>Nome de exibição</small>
            {editingName ? (
              <div className="profile-name-edit">
                <Input aria-label="Nome de exibição" value={name} onChange={e => setName(e.target.value)} autoFocus
                       onKeyDown={e => {
                         if (e.key === 'Enter') commitName();
                         if (e.key === 'Escape') setEditingName(false);
                       }}/>
                <Button size="icon" disabled={!name.trim() || nameSave.isPending} aria-label="Salvar"
                        onClick={commitName}>
                  {nameSave.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> :
                    <Check aria-hidden="true"/>}
                </Button>
                <Button size="icon" variant="ghost" aria-label="Cancelar edição do nome"
                        onClick={() => setEditingName(false)}><X aria-hidden="true"/></Button>
              </div>
            ) : (
              <button type="button" className="profile-name-display" onClick={() => {
                setName(me?.name || '');
                setEditingName(true);
              }}>
                <span>{me?.name || 'Definir nome'}</span><Pencil aria-hidden="true"/>
              </button>
            )}
            <span className="profile-visibility"><i aria-hidden="true"/>
              {me?.showcase_public ? 'Vitrine pública' : 'Vitrine privada'}
            </span>
          </div>
          {me?.avatar_url && <ProfilePhotoRemoveButton size="icon" className="profile-avatar-remove"
                                                       aria-label="Remover foto de perfil"/>}
        </header>

        <section className="profile-menu-section" aria-labelledby="profile-table-title">
          <div className="profile-menu-section-heading">
            <div><Sparkles aria-hidden="true"/><span><b id="profile-table-title">Sua mesa, do seu jeito</b>
              <small>Preferências aplicadas na próxima mão.</small></span></div>
          </div>
          {menuOpened && <DeckPicker className="profile-deck-setting" deckVariant={me?.deck_variant}/>}
        </section>

        <section className="profile-wallet" aria-label="Seus saldos">
          <div className="profile-wallet-heading"><WalletCards aria-hidden="true"/><b>Seus saldos</b></div>
          <div className="profile-balances">
            <span>Fichas <b>{chipsExact(me?.sandbox_balance ?? 0)}</b></span>
            {REAL_MONEY_UI_ENABLED && <span>Dinheiro real <b>{moneyExact(me?.game_balance)}</b></span>}
          </div>
          <Button type="button" variant="ghost" className="profile-wallet-link" render={<Link href="/store"/>}>
            <ShoppingBag aria-hidden="true"/> <span><b>Loja</b><small>Reações, baralhos e fichas</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
        </section>

        <nav className="profile-menu-links" aria-label="Detalhes do perfil">
          <Button type="button" variant="ghost" render={<Link href="/player-profile"/>}>
            <UserRound aria-hidden="true"/><span><b>Editar perfil</b><small>Vitrine, conquistas em destaque e mais</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
          <Button type="button" variant="ghost" aria-label="Seu jogo" onClick={() => setSelfHudOpen(true)}>
            <Activity aria-hidden="true"/><span><b>Seu jogo</b><small>Estatísticas e estilo na mesa</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
        </nav>

        <Button variant="ghost" className="profile-menu-logout" loading={loggingOut} onClick={signOut}>
          {!loggingOut && <LogOut aria-hidden="true"/>} {loggingOut ? 'Saindo…' : 'Sair da conta'}
        </Button>
      </div>
    </PopoverContent>
  </Popover>
    <SelfHudDialog open={selfHudOpen} onOpenChangeAction={setSelfHudOpen}/>
  </>;
}
