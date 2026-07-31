'use client';

import type {LucideIcon} from 'lucide-react';
import {Award, BookOpen, ChevronLeft, Club, History, ShoppingBag, Trophy} from 'lucide-react';
import Link from 'next/link';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';

type MainRoute = 'guide' | 'leaderboard' | 'achievements' | 'hands' | 'store';

const MAIN_ROUTES: {href: string; label: string; route: MainRoute; icon: LucideIcon}[] = [
  {href: '/guide', label: 'Guia', route: 'guide', icon: BookOpen},
  {href: '/leaderboard', label: 'Ranking', route: 'leaderboard', icon: Trophy},
  {href: '/achievements', label: 'Conquistas', route: 'achievements', icon: Award},
  {href: '/hands', label: 'Mãos', route: 'hands', icon: History},
  {href: '/store', label: 'Loja', route: 'store', icon: ShoppingBag},
];

export function AppPageNav({authed, current, rewardReady = false}: {
  authed: boolean;
  current?: MainRoute;
  rewardReady?: boolean;
}) {
  return (
    <nav className="app-nav shell" aria-label="Navegação principal">
      <Link href="/" className="brand" aria-label="CTech Poker — início">
        <span className="brand-mark"><Club/></span>
        <span className="brand-name">CTech <b>Poker</b></span>
      </Link>
      {authed ? (
        <div className="header-right">
          {MAIN_ROUTES.map(({href, label, route, icon: Icon}) => (
            <Link key={route} href={href} aria-current={current === route ? 'page' : undefined}
                  className={route === 'store' ? 'app-nav-store-link' : undefined}
                  title={route === 'store' && rewardReady ? 'Loja — recompensa diária disponível' : label}>
              <Icon aria-hidden="true"/>
              <span className="header-right-label">{label}</span>
              {route === 'store' && rewardReady && <>
                <span className="app-nav-reward-dot" aria-hidden="true"/>
                <span className="sr-only"> — recompensa diária disponível</span>
              </>}
            </Link>
          ))}
          <ProfileMenu/>
        </div>
      ) : (
        <Link href="/" className="app-nav-public-back"><ChevronLeft aria-hidden="true"/> Voltar</Link>
      )}
    </nav>
  );
}

export function AppPageHeader({
  icon: Icon,
  eyebrow,
  title,
  description,
  backHref,
  backLabel = 'Lobby',
}: {
  icon: LucideIcon;
  eyebrow: string;
  title: string;
  description: React.ReactNode;
  backHref?: string;
  backLabel?: string;
}) {
  return (
    <div className="page-heading-shell">
      {backHref && (
        <Link href={backHref} className="page-back-link">
          <ChevronLeft aria-hidden="true"/> {backLabel}
        </Link>
      )}
      <header className="page-heading">
        <span className="page-heading-icon"><Icon aria-hidden="true"/></span>
        <small>{eyebrow}</small>
        <h1>{title}</h1>
        <p>{description}</p>
      </header>
    </div>
  );
}
