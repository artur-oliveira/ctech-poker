'use client';

import type {LucideIcon} from 'lucide-react';
import {Award, BookOpen, ChevronLeft, History, LayoutGrid, MoreHorizontal, ShoppingBag, Trophy, Users} from 'lucide-react';
import Link from 'next/link';
import type {ReactNode} from 'react';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {PokerLogo} from '@/components/PokerLogo';
import {PeopleNavBadge} from '@/components/social/PeopleNavBadge';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';

type MainRoute = 'lobby' | 'people' | 'guide' | 'leaderboard' | 'achievements' | 'hands' | 'store';

const MAIN_ROUTES: {href: string; label: string; route: MainRoute; icon: LucideIcon}[] = [
  {href: '/lobby', label: 'Lobby', route: 'lobby', icon: LayoutGrid},
  {href: '/people', label: 'Pessoas', route: 'people', icon: Users},
  {href: '/guide', label: 'Guia', route: 'guide', icon: BookOpen},
  {href: '/leaderboard', label: 'Ranking', route: 'leaderboard', icon: Trophy},
  {href: '/achievements', label: 'Conquistas', route: 'achievements', icon: Award},
  {href: '/hands', label: 'Mãos', route: 'hands', icon: History},
  {href: '/store', label: 'Loja', route: 'store', icon: ShoppingBag},
];

// The mobile tab bar surfaces these three directly (Lobby is home base,
// Pessoas/Loja carry live badges) and tucks the rest behind "Mais" — seven
// icons plus the profile avatar never fit a phone-width row at a 44px touch
// floor, and forcing them all in stopped being a real IA choice once nobody
// could tell which ones actually mattered.
const TAB_BAR_PRIMARY: MainRoute[] = ['lobby', 'people', 'store'];

function routeBadgeClass(route: MainRoute) {
  return route === 'store' ? 'app-nav-store-link' : route === 'people' ? 'app-nav-people-link' : undefined;
}

export function AppPage({authed, current, rewardReady = false, children, footer = true}: {
  authed: boolean;
  current?: MainRoute;
  rewardReady?: boolean;
  children: ReactNode;
  footer?: boolean;
}) {
  return <main className={authed ? 'app-page has-tab-bar' : 'app-page'}>
    <AppPageNav authed={authed} current={current} rewardReady={rewardReady}/>
    {children}
    {footer && <AppPageFooter authed={authed}/>}
  </main>;
}

export function AppPageBody({children, className = ''}: {children: ReactNode; className?: string}) {
  return <section className={`content-page shell ${className}`.trim()}>{children}</section>;
}

export function AppPageNav({authed, current, rewardReady = false}: {
  authed: boolean;
  current?: MainRoute;
  rewardReady?: boolean;
}) {
  return <>
    <nav className="app-nav shell" aria-label="Navegação principal">
      <Link href="/" className="brand" aria-label="CTech Poker - início">
        <span className="brand-mark"><PokerLogo priority/></span>
        <span className="brand-name">CTech <b>Poker</b></span>
      </Link>
      {authed ? <div className="header-right">
        {/* Hidden below 600px — the tab bar below takes over as the mobile
           destination for these same routes. */}
        <div className="app-nav-routes">
          {MAIN_ROUTES.map(({href, label, route, icon: Icon}) => <Link key={route} href={href}
            aria-current={current === route ? 'page' : undefined}
            className={routeBadgeClass(route)}
            title={route === 'store' && rewardReady ? 'Loja - recompensa diária disponível' : label}>
            <Icon aria-hidden="true"/><span className="header-right-label">{label}</span>
            {route === 'store' && rewardReady && <><span className="app-nav-reward-dot" aria-hidden="true"/>
              <span className="sr-only"> - recompensa diária disponível</span></>}
            {route === 'people' && <PeopleNavBadge/>}
          </Link>)}
        </div>
        <ProfileMenu/>
      </div> : <Link href="/" className="app-nav-public-back"><ChevronLeft aria-hidden="true"/> Voltar</Link>}
    </nav>
    {authed && <AppTabBar current={current} rewardReady={rewardReady}/>}
  </>;
}

function AppTabBar({current, rewardReady}: {current?: MainRoute; rewardReady: boolean}) {
  const primary = MAIN_ROUTES.filter(({route}) => TAB_BAR_PRIMARY.includes(route));
  const secondary = MAIN_ROUTES.filter(({route}) => !TAB_BAR_PRIMARY.includes(route));
  const moreActive = secondary.some(({route}) => route === current);

  return <nav className="app-tab-bar" aria-label="Navegação rápida">
    {primary.map(({href, label, route, icon: Icon}) => <Link key={route} href={href}
      aria-current={current === route ? 'page' : undefined} className={routeBadgeClass(route)}>
      <span className="app-tab-bar-icon">
        <Icon aria-hidden="true"/>
        {route === 'store' && rewardReady && <><span className="app-nav-reward-dot" aria-hidden="true"/>
          <span className="sr-only"> - recompensa diária disponível</span></>}
        {route === 'people' && <PeopleNavBadge/>}
      </span>
      <span>{label}</span>
    </Link>)}
    <Popover>
      <PopoverTrigger render={<button type="button" className={moreActive ? 'is-active' : undefined}/>}>
        <MoreHorizontal aria-hidden="true"/><span>Mais</span>
      </PopoverTrigger>
      <PopoverContent className="app-tab-bar-menu" side="top" align="center" aria-label="Mais opções">
        {secondary.map(({href, label, route, icon: Icon}) => <Link key={route} href={href}
          className="social-actions-item" aria-current={current === route ? 'page' : undefined}>
          <Icon aria-hidden="true"/>{label}
        </Link>)}
      </PopoverContent>
    </Popover>
  </nav>;
}

export function AppPageHeader({icon: Icon, eyebrow, title, description, actions}: {
  icon: LucideIcon;
  eyebrow: string;
  title: string;
  description: ReactNode;
  actions?: ReactNode;
}) {
  return <div className="page-heading-shell">
    <div className="page-heading-row">
      <header className="page-heading">
        <span className="page-heading-icon"><Icon aria-hidden="true"/></span>
        <small>{eyebrow}</small>
        <h1>{title}</h1>
        <p>{description}</p>
      </header>
      {actions && <div className="page-heading-actions">{actions}</div>}
    </div>
  </div>;
}

export function AppPageFooter({authed}: {authed: boolean}) {
  return <footer className="app-page-footer shell">
    <div><span className="brand-mark" aria-hidden="true"><PokerLogo/></span>
      <p><strong>CTech Poker</strong><span>Jogo social, transparente e responsável.</span></p></div>
    <nav aria-label="Links do rodapé">
      <Link href="/guide">Como jogar</Link>
      <Link href="/poker-rules">Regras</Link>
      <Link href={authed ? '/lobby' : '/'}>{authed ? 'Voltar ao lobby' : 'Início'}</Link>
    </nav>
  </footer>;
}
