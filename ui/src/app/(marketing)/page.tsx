'use client';
// The landing's live table preview shares the renderer cascade without making it global.
import '../renderer.css';
import Image from 'next/image';
import Link from 'next/link';
import {
  ArrowRight,
  Award,
  Coins,
  Gift,
  History,
  MessageCircleMore,
  NotebookPen,
  Repeat2,
  ShieldCheck,
  Sparkles,
  Trophy,
  UserRoundSearch,
  Users,
  Zap
} from 'lucide-react';
import {startOAuthFlow} from '@/lib/auth/oauth';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import {cardPath} from '@/lib/cards';
import {achievementDescription, achievementExample, achievementLabel} from '@/lib/achievements';
import {PokerLogo} from '@/components/PokerLogo';
import React from 'react';

// A curated, static preview, not fetched from the catalog. The landing page
// is public and shouldn't take an API dependency just to tease four cards.
const LANDING_ACHIEVEMENTS = [
  'win_category_royal_flush', 'giant_slayer', 'bluff', 'wins'
];

const features = [
  {
    icon: Zap,
    title: 'Mesa ao Vivo',
    body: 'Jogue em mesas rápidas em tempo real'
  },
  {
    icon: ShieldCheck,
    title: 'Embaralhamento seguro',
    body: 'O baralho usa um sistema que permite verificar se tudo foi justo. O histórico fica gravado para conferência.'
  },
  {
    icon: Users,
    title: 'Convide seus amigos',
    body: 'Crie uma mesa privada para 2 a 9 pessoas, escolha os stakes e jogue a vontade.'
  },
  {
    icon: History,
    title: 'Melhores momentos',
    body: 'Veja cada mão novamnete, compartilhe aquela virada incrível ou aquela derrota improvável.'
  },
  {
    icon: Trophy,
    title: 'Sua história',
    body: 'Estatísticas, estilo de jogo, ranking, vitórias e conquistas tornam sua experiência ainda melhor.'
  },
  {
    icon: Award,
    title: 'Conquistas e recompensas',
    body: 'Ganhe um bônus diário e desbloqueie conquistas conforme você joga'
  }
];

export default function Home() {
  return <main className="landing">
    <nav className="nav shell">
      <Link href="/" className="brand">
        <span className="brand-mark"><PokerLogo priority/></span>
        <span>CTech <b>Poker</b></span>
      </Link>
      <div className="nav-links">
        <Link href="#novidades">Novidades</Link>
        <Link href="#experience">Por que jogar</Link>
        <Link href="#achievements">Conquistas</Link>
        <Link href="/poker-rules">Regras</Link>
        <Link href="/guide">Guia</Link>
        <Link href="/leaderboard">Ranking</Link>
        <Button variant="ghost" onClick={() => startOAuthFlow()}>Entrar</Button>
      </div>
    </nav>
    <section className="hero shell">
      <div className="hero-copy">
        <span className="hero-kicker">
          <Sparkles aria-hidden="true"/>
          Sua próxima mesa já está pronta
        </span>
        <h1>Chame seus amigos para jogar poker.</h1>
        <p>
          Jogue Texas Hold&apos;em online com seus amigos e guarde cada mão na história.
          Tudo no navegador, sem instalar nada.
        </p>
        <div className="hero-actions">
          <Button size="lg" onClick={() => startOAuthFlow('/lobby')}>Jogar agora <ArrowRight/></Button>
          <Button variant="outline" size="lg" render={<Link href="#novidades"/>}>Conhecer recursos</Button>
        </div>
        <div className="trust">
          <span>
            <i/> Fichas sandbox
          </span>
          <span>2–9 jogadores</span>
          <span>Sem download</span>
          <span>Embaralhamento justo </span>
        </div>
      </div>
      <HeroTable/>
    </section>
    <section id="novidades" className="landing-new shell">
      <header className="landing-new-heading">
        <h2>Mais recursos para suas partidas</h2>
        <p>
          A experiência do poker vai além da aposta: entender os adversários, interagir com os amigos em tempo real
          e compartilhar os melhores momentos que você terá jogando o CTech Poker.
        </p>
      </header>

      <article className="landing-story landing-story-social">
        <div className="landing-story-copy">
          <MessageCircleMore aria-hidden="true"/>
          <h3>Mais formas de interagir na mesa.</h3>
          <p>
            Reações e conversa a vontade. Aplauda as melhores mãos, ria dos piores blefes e registre notas
            privadas sobre os adversários. Em mesas habilitadas, um all-in ainda pode ter dois desfechos com
            <em>Run it twice</em>.
          </p>
          <ul className="landing-feature-list">
            <li><MessageCircleMore aria-hidden="true"/> Reações ao vivo</li>
            <li><NotebookPen aria-hidden="true"/> Notas pessoais</li>
            <li><Repeat2 aria-hidden="true"/> <em>Run it twice</em></li>
          </ul>
        </div>
        <RealScreen src="/og/table.webp"
                    alt="Mesa real do CTech Poker com jogadores, cartas comunitárias e controles de ação"
                    label="Mesa ao vivo" className="landing-screen-table"/>
      </article>

      <article className="landing-story landing-story-insight">
        <div className="landing-story-gallery" aria-label="Telas reais de perfil e replay">
          <RealScreen src="/og/profile.webp" alt="Perfil público real com estilo de jogo e conquistas"
                      label="Seu perfil"/>
          <RealScreen src="/og/hand-replay.webp" alt="Replay real de uma mão de poker ação por ação"
                      label="Replay completo"/>
        </div>
        <div className="landing-story-copy">
          <UserRoundSearch aria-hidden="true"/>
          <h3>Conheça que tipo de jogador você é.</h3>
          <p>
            Mostre seu estilo de jogo e suas melhores conquistas no seu perfil.
            Compartilhe o replay das suas melhores (ou piores) mãos com seus amigos.
          </p>
          <Link href="/hands">Ver histórico e replays<ArrowRight/></Link>
        </div>
      </article>

      <aside className="landing-reward">
        <span className="landing-reward-icon">
          <Gift aria-hidden="true"/>
        </span>
        <div>
          <h3>Volte diariamente para resgatar fichas grátis.</h3>
          <p>
            As fichas grátis podem ser obtidas diariamente na loja.
            Você também pode comprar fichas com Pix ou conquistá-las da forma mais interessante: Jogando
          </p>
        </div>
        <Button variant="outline" onClick={() => startOAuthFlow('/store')}>
          Ver fichas
          <Coins aria-hidden="true"/>
        </Button>
      </aside>
    </section>
    <section id="experience" className="experience shell">
      <header>
        <h2>Sua melhor experiência de poker online.</h2>
        <p>
          Entre, compartilhe seus amigos e jogue, ação rápida e transparência em cada mão
        </p>
      </header>
      <div className="feature-grid">
        {features.map(({icon: Icon, title, body}, i) =>
          <article key={title}
                   style={{'--delay': `${i * 90}ms`} as React.CSSProperties}>
            <div><Icon/></div>
            <h3>{title}</h3>
            <p>{body}</p>
          </article>)}
      </div>
    </section>
    <section id="achievements" className="achievements-teaser shell">
      <div className="achievements-teaser-copy">
        <h2>Conquistas para os momentos que valem a pena lembrar.</h2>
        <p>Blefes que funcionam, all-ins decisivos, combinações estatisticamente improváveis:
          cada conquista premia seu estilo de jogo com até 5 estrelas de maestria.
        </p>
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
        <h2>Do lobby ao showdown, em qualquer lugar.</h2>
        <p>O CTech poker pode ser jogado em qualquer dispositivo mantendo todas suas funcionalidades.</p>
        <Link href="/guide">Acessar o guia completo <ArrowRight/></Link>
      </div>
      <div className="showcase-frame">
        <div className="showcase-inner">
          <div className="browser-chrome">
            <span/>
            <span/>
            <span/>
            <small>poker.aoctech.app/table</small>
          </div>
          <Image src="/guide/table-flop.webp"
                 alt="Mesa real do CTech Poker em andamento, com cartas comunitárias e barra de ações"
                 width={1280} height={800}/>
        </div>
        <figure className="showcase-peek">
          <Image src="/guide/lobby.webp" alt="Lobby do CTech Poker com mesas agrupadas por stake" width={640}
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
      <div className="brand"><span className="brand-mark"><PokerLogo/></span><span>CTech <b>Poker</b></span></div>
      <div className="footer-content">
        <p>Jogue com responsabilidade. © {new Date().getFullYear()} A O CARVALHO TECH</p>
        <nav>
          <a href="https://accounts.aoctech.app/products/poker" target="_blank" rel="noreferrer">Termos de Uso</a>
          <a href="https://accounts.aoctech.app/products/poker-privacy" target="_blank" rel="noreferrer">Política de
            privacidade</a>
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
        <div className="table-logo"><PokerLogo size={14}/> CTECH</div>
      </div>
      {[['Kely', '1.820', 'top'], ['Você', '3.240', 'bottom'], ['Wellington', '980', 'left'], ['Thiago', '2.100', 'right']].map(([name, chips, pos]) =>
        <div className={`demo-seat ${pos}`} key={name}><span
          className="avatar">{name[0]}</span><span><b>{name}</b><small>{chips}</small></span>{name === 'Você' &&
            <div className="hole"><Image src={cardPath('As')} alt="Ás de espadas" width={42} height={59}/><Image
                src={cardPath('Ah')} alt="Ás de copas" width={42} height={59}/></div>}</div>)}
      <div className="chip-orbit chip-a"/>
      <div className="chip-orbit chip-b"/>
      <span className="hero-reaction" role="img" aria-label="Aplausos">👏</span>
      <span className="hero-style"><UserRoundSearch aria-hidden="true"/> Equilibrado</span>
    </div>
  </div>;
}

function RealScreen({src, alt, label, className = ''}: {
  src: string;
  alt: string;
  label: string;
  className?: string;
}) {
  return <figure className={`landing-real-screen ${className}`}>
    <div className="landing-screen-chrome" aria-hidden="true"><span/><span/><span/><small>{label}</small></div>
    <Image src={src} alt={alt} width={1200} height={630}/>
  </figure>;
}
