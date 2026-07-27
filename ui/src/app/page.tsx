'use client';
import Image from 'next/image';
import Link from 'next/link';
import {ArrowRight, Award, Club, History, ShieldCheck, Trophy, Users, Zap} from 'lucide-react';
import {startOAuthFlow} from '@/lib/auth/oauth';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import {cardPath} from '@/lib/cards';
import {achievementDescription, achievementExample, achievementLabel} from '@/lib/achievements';
import React from "react";

// A curated, static preview — not fetched from the catalog. The landing page
// is public and shouldn't take an API dependency just to tease four cards.
const LANDING_ACHIEVEMENTS = ['win_category_royal_flush', 'giant_slayer', 'bluff', 'wins'];

const features = [
  {
    icon: Zap,
    title: 'Ação em Tempo Real',
    body: 'Mesas rápidas via WebSockets com servidor autoritativo, sincronização de turnos e reconexão automática.'
  },
  {
    icon: Users,
    title: 'Salas Públicas e Privadas',
    body: 'Crie salas personalizadas para 2 a 9 jogadores com link direto de convite ou jogue nas mesas públicas do lobby.'
  },
  {
    icon: ShieldCheck,
    title: 'Provably Fair SHA-256',
    body: 'Baralho auditável com commit hash criptográfico e revelação de seed após cada mão para transparência absoluta.'
  },
  {
    icon: History,
    title: 'Replay e Histórico de Mãos',
    body: 'Reviva cada jogada ação por ação em tela cheia e exporte seus históricos nos formatos texto e JSON.'
  },
  {
    icon: Trophy,
    title: 'Ranking e Bônus Diário',
    body: 'Suba no Hall da Fama da comunidade com base em suas vitórias e resgate seu bônus diário de fichas sandbox.'
  },
  {
    icon: Award,
    title: 'Conquistas Progressivas',
    body: 'Sistema de progresso com 5 níveis de estrelas por categoria de mão, blefes, potes acumulados e sequência de vitórias.'
  }
];

export default function Home() {
  return <main className="landing">
    <nav className="nav shell">
      <Link href="/" className="brand">
        <span className="brand-mark"><Club/></span>
        <span>CTech <b>Poker</b></span>
      </Link>
      <div className="nav-links">
        <Link href="#experience">Recursos</Link>
        <Link href="#achievements">Conquistas</Link>
        <Link href="/poker-rules">Regras</Link>
        <Link href="/guide">Guia</Link>
        <Link href="/leaderboard">Ranking</Link>
        <Button variant="ghost" onClick={() => startOAuthFlow()}>Entrar</Button>
      </div>
    </nav>
    <section className="hero shell">
      <div className="hero-copy">
        <h1>Sua mesa de poker, <em>sempre pronta no navegador.</em></h1>
        <p>Texas Hold&apos;em em tempo real com fichas sandbox. Entre em mesas públicas, crie salas privadas de 2 a 9 lugares para jogar com amigos e acompanhe seu desempenho no ranking.</p>
        <div className="hero-actions">
          <Button size="lg" onClick={() => startOAuthFlow('/lobby')}>Jogar agora <ArrowRight/></Button>
          <Button variant="outline" size="lg" render={<Link href="#experience"/>}>Conhecer recursos</Button>
        </div>
        <div className="trust">
          <span><i/> Sandbox 100% Grátis</span>
          <span>2–9 jogadores</span>
          <span>Provably Fair SHA-256</span>
          <span>Replay em HD</span>
        </div>
      </div>
      <HeroTable/>
    </section>
    <section id="experience" className="experience shell">
      <header>
        <h2>Uma experiência completa de poker</h2>
        <p>Desenvolvido para oferecer partidas fluidas, transparência total e uma interface rica que coloca você no centro da ação.</p>
      </header>
      <div className="feature-grid">
        {features.map(({icon: Icon, title, body}, i) => <article key={title}
                                                                 style={{'--delay': `${i * 90}ms`} as React.CSSProperties}>
          <div><Icon/></div>
          <h3>{title}</h3>
          <p>{body}</p>
        </article>)}
      </div>
    </section>
    <section id="achievements" className="achievements-teaser shell">
      <div className="achievements-teaser-copy">
        <h2>Suba de nível a cada mão.</h2>
        <p>Blefes que funcionam, all-ins decisivos, combinações raras na mesa — cada conquista premia seu estilo de jogo com até 5 estrelas de maestria.</p>
        <Link href="/achievements">Ver catálogo de conquistas <ArrowRight/></Link>
      </div>
      <div className="achievements-teaser-grid">
        {LANDING_ACHIEVEMENTS.map((key, i) =>
          <article key={key}
                   style={{'--delay': `${i * 90}ms`} as React.CSSProperties}>
            <div className="achievements-teaser-art" aria-hidden="true">
              {achievementExample(key).map((card, ci) => <PlayingCard key={`${card}-${ci}`} card={card} index={ci}
                                                                      size="hole"/>)}
            </div>
            <b>{achievementLabel(key)}</b>
            <small>{achievementDescription(key)}</small>
          </article>)}
      </div>
    </section>
    <section id="showcase" className="showcase shell">
      <div className="showcase-copy">
        <h2>O poker de verdade, sem instalar nada.</h2>
        <p>Direto no seu navegador: cartas comunitárias, cronômetro de ação, atalhos de teclado (F, C, P, R), força da mão estimada e histórico com prova de integridade. Sem cliente pesado para atualizar.</p>
        <Link href="/guide">Ver o guia completo da mesa <ArrowRight/></Link>
      </div>
      <div className="showcase-frame">
        <div className="showcase-inner">
          <div className="browser-chrome">
            <span/>
            <span/>
            <span/>
            <small>poker.aoctech.app/table</small>
          </div>
          <Image src="/guide/table-flop.png"
                 alt="Mesa real do CTech Poker em andamento, com cartas comunitárias e barra de ações"
                 width={1280} height={800}/>
        </div>
        <figure className="showcase-peek">
          <Image src="/guide/lobby.png" alt="Lobby do CTech Poker com mesas agrupadas por stake" width={640}
                 height={400}/>
        </figure>
      </div>
    </section>
    <section className="cta shell">
      <div>
        <h2>Abra sua mesa em segundos.</h2>
        <p>Entre com sua conta CTech e jogue de graça no sandbox com seus amigos.</p>
      </div>
      <Button variant="light" size="lg" onClick={() => startOAuthFlow('/lobby')}>
        Jogar agora <ArrowRight/>
      </Button>
    </section>
    <footer className="footer shell">
      <div className="brand"><span className="brand-mark"><Club/></span><span>CTech <b>Poker</b></span></div>
      <div className="footer-content">
        <p>Jogue com responsabilidade. © {new Date().getFullYear()} A O CARVALHO TECH</p>
        <nav>
          <a href="https://accounts.aoctech.app/products/poker" target="_blank" rel="noreferrer">Termos de Uso</a>
          <a href="https://accounts.aoctech.app/products/poker-privacy" target="_blank" rel="noreferrer">Política de privacidade</a>
          <a href="https://accounts.aoctech.app/legal" target="_blank" rel="noreferrer">Central Jurídica</a>
        </nav>
      </div>
    </footer>
  </main>;
}

function HeroTable() {
  return <div className="hero-visual" aria-label="Prévia de uma mesa de poker">
    <div className="ambient ambient-one"/>
    <div className="ambient ambient-two"/>
    <div className="poker-table">
      <div className="rail"/>
      <div className="felt"><span className="pot">POTE <b>2.450</b></span>
        <div className="community">{['Th', 'Js', 'Qd'].map((c, i) => <Image key={c}
                                                                             src={cardPath(c)}
                                                                             alt="" width={70}
                                                                             height={98}
                                                                             style={{'--i': i} as React.CSSProperties}/>)}<span
          className="card-placeholder"/><span className="card-placeholder"/></div>
        <div className="table-logo"><Club/> CTECH</div>
      </div>
      {[['Kely', '1.820', 'top'], ['Você', '3.240', 'bottom'], ['Wellington', '980', 'left'], ['Thiago', '2.100', 'right']].map(([name, chips, pos]) =>
        <div className={`demo-seat ${pos}`} key={name}><span
          className="avatar">{name[0]}</span><span><b>{name}</b><small>{chips}</small></span>{name === 'Você' &&
            <div className="hole"><Image src={cardPath('As')} alt="Ás de espadas" width={42} height={59}/><Image
                src={cardPath('Ah')} alt="Ás de copas" width={42} height={59}/></div>}</div>)}
      <div className="chip-orbit chip-a"/>
      <div className="chip-orbit chip-b"/>
    </div>
  </div>;
}
