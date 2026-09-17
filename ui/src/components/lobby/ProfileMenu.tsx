'use client';
import {useState} from 'react';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {Activity, ChevronRight, LogOut, ShoppingBag, UserRound, WalletCards} from 'lucide-react';
import {getMe, type WalletMode} from '@/lib/api/player';
import {endSession, logout} from '@/lib/auth/oauth';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {Button} from '@/components/ui/button';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {SelfHudDialog} from '@/components/lobby/SelfHudDialog';
import {chipsExact, moneyExact} from '@/lib/chips';
import {pushNotification} from '@/lib/notify';
import {availableWalletMode, REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';

/** `logout()` revokes the refresh token under a 3 s deadline and only then
 * redirects through the IdP. Past that the redirect itself is what stalled, so
 * the toast offers the direct end-session exit. Deliberately never cleared: a
 * successful logout navigates away and takes the timer with it. */
const LOGOUT_STALL_MS = 4_000;

function formatChips(amount?: number) {
  return `${chipsExact(amount ?? 0)} fichas`;
}

/**
 * The header shortcut, not the editor. Everything dense — name, photo, deck,
 * mode, and the whole public showcase — lives on `/player-profile`, so this
 * panel is one thing: who you are, what you hold, and where to go next. It is
 * opened many times a session and reads nothing beyond the profile the shell
 * already has.
 */
export function ProfileMenu() {
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  const [selfHudOpen, setSelfHudOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  function signOut() {
    if (loggingOut) return;
    setLoggingOut(true);
    window.setTimeout(() => pushNotification(
      'A saída está demorando mais que o normal.', 'error',
      [{label: 'Sair agora', run: () => endSession()}]
    ), LOGOUT_STALL_MS);
    void logout();
  }

  // Coerce to chips whenever the real-money UI is gated off, so the pill and
  // the balance label never imply real money is active.
  const walletMode: WalletMode = availableWalletMode(me?.wallet_mode);
  const balanceLabel = walletMode === 'real' ? moneyExact(me?.game_balance) : formatChips(me?.sandbox_balance);

  return <><Popover>
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
          <div className="profile-menu-avatar">
            <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url} size={64}/>
          </div>
          <div className="profile-menu-identity-copy">
            <small>Nome de exibição</small>
            <strong className="profile-name-readout">{me?.name || 'Sem nome ainda'}</strong>
            <span className="profile-visibility"><i aria-hidden="true"/>
              {me?.showcase_public ? 'Vitrine pública' : 'Vitrine privada'}
            </span>
          </div>
        </header>

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
            <UserRound aria-hidden="true"/><span><b>Editar perfil</b><small>Nome, foto, baralho e vitrine</small></span>
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
