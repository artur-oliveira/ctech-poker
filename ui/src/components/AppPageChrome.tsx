'use client';

import type {LucideIcon} from 'lucide-react';
import {Award, BookOpen, ChevronLeft, Club, History, ShoppingBag, Trophy} from 'lucide-react';
import Link from 'next/link';
import type {ReactNode} from 'react';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';

type MainRoute = 'guide' | 'leaderboard' | 'achievements' | 'hands' | 'store';

const MAIN_ROUTES: {href: string; label: string; route: MainRoute; icon: LucideIcon}[] = [
  {href: '/guide', label: 'Guia', route: 'guide', icon: BookOpen},
  {href: '/leaderboard', label: 'Ranking', route: 'leaderboard', icon: Trophy},
  {href: '/achievements', label: 'Conquistas', route: 'achievements', icon: Award},
  {href: '/hands', label: 'Mãos', route: 'hands', icon: History},
  {href: '/store', label: 'Loja', route: 'store', icon: ShoppingBag},
];

export function AppPage({authed, current, rewardReady = false, children, footer = true}: {
  authed: boolean;
  current?: MainRoute;
  rewardReady?: boolean;
  children: ReactNode;
  footer?: boolean;
}) {
  return <main className="app-page">
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
  return <nav className="app-nav shell" aria-label="Navegação principal">
    <Link href="/" className="brand" aria-label="CTech Poker — início">
      <span className="brand-mark"><Club/></span>
      <span className="brand-name">CTech <b>Poker</b></span>
    </Link>
    {authed ? <div className="header-right">
      {MAIN_ROUTES.map(({href, label, route, icon: Icon}) => <Link key={route} href={href}
        aria-current={current === route ? 'page' : undefined}
        className={route === 'store' ? 'app-nav-store-link' : undefined}
        title={route === 'store' && rewardReady ? 'Loja — recompensa diária disponível' : label}>
        <Icon aria-hidden="true"/><span className="header-right-label">{label}</span>
        {route === 'store' && rewardReady && <><span className="app-nav-reward-dot" aria-hidden="true"/>
          <span className="sr-only"> — recompensa diária disponível</span></>}
      </Link>)}
      <ProfileMenu/>
    </div> : <Link href="/" className="app-nav-public-back"><ChevronLeft aria-hidden="true"/> Voltar</Link>}
  </nav>;
}

export function AppPageHeader({icon: Icon, eyebrow, title, description, backHref, backLabel = 'Lobby', actions}: {
  icon: LucideIcon;
  eyebrow: string;
  title: string;
  description: ReactNode;
  backHref?: string;
  backLabel?: string;
  actions?: ReactNode;
}) {
  return <div className="page-heading-shell">
    {backHref && <Link href={backHref} className="page-back-link">
      <ChevronLeft aria-hidden="true"/> {backLabel}
    </Link>}
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
    <div><span className="brand-mark" aria-hidden="true"><Club/></span>
      <p><strong>CTech Poker</strong><span>Jogo social, transparente e responsável.</span></p></div>
    <nav aria-label="Links do rodapé">
      <Link href="/guide">Como jogar</Link>
      <Link href="/poker-rules">Regras</Link>
      <Link href={authed ? '/lobby' : '/'}>{authed ? 'Voltar ao lobby' : 'Início'}</Link>
    </nav>
  </footer>;
}
