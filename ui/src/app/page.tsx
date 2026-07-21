'use client'
import Image from 'next/image';
import Link from 'next/link';
import {ArrowRight, Club, ShieldCheck, Sparkles, Trophy, Users, Zap} from 'lucide-react';
import {startOAuthFlow} from '@/lib/auth/oauth'
import {Button} from '@/components/ui/button'

const features = [{
  icon: Zap,
  title: 'Ação em cada jogada',
  body: 'Cartas, fichas e potes se movimentam com fluidez para você acompanhar tudo sem perder a clareza.'
}, {
  icon: Users,
  title: 'Feito para você',
  body: 'Entre em mesas públicas ou crie uma sala privada para jogar com quem você quiser, onde estiver.'
}, {
  icon: Trophy,
  title: 'Sempre há uma próxima meta',
  body: 'Ranking, conquistas por estrelas e recompensas sandbox tornam cada sessão memorável.'
}, {
  icon: ShieldCheck,
  title: 'Jogo transparente',
  body: 'Servidor autoritativo, baralho criptográfico e equity privada para uma experiência justa.'
}]
export default function Home() {
  return <main className="landing">
    <nav className="nav shell"><Link href="/" className="brand"><span
      className="brand-mark"><Club/></span><span>CTech <b>Poker</b></span></Link>
      <div className="nav-links"><Link href="#experience">Experiência</Link><Link href="/leaderboard">Ranking</Link>
        <Button variant="ghost" onClick={() => startOAuthFlow()}>Entrar</Button>
      </div>
    </nav>
    <section className="hero shell">
      <div className="hero-copy">
        <div className="eyebrow"><span/> Poker do seu jeito</div>
        <h1>A mesa está <em>pronta.</em><br/>Só falta você.</h1><p>Texas Hold&apos;em prático, social, onde você
        estiver. Jogue com fichas de sandbox ou com dinheiro real, reúna seus amigos e transforme qualquer lugar na sua
        mesa.</p>
        <div className="hero-actions">
          <Button size="lg" onClick={() => startOAuthFlow('/lobby')}>Jogar agora <ArrowRight/></Button>
          <Button variant="outline" render={<Link href="#experience"/>}>Conhecer o jogo</Button></div>
        <div className="trust"><span><i/> Sandbox grátis</span><span>2–9 jogadores</span><span>Responsivo</span></div>
      </div>
      <HeroTable/></section>
    <section id="experience" className="experience shell">
      <header><span>Mais do que um jogo de poker</span><h2>Decisões claras. Jogo envolvente.</h2><p>Interações naturais,
        informação no momento certo e movimentos suaves que ajudam você a acompanhar cada jogada.</p></header>
      <div className="feature-grid">{features.map(({icon: Icon, title, body}, i) => <article key={title}
                                                                                             style={{'--delay': `${i * 90}ms`} as React.CSSProperties}>
        <div><Icon/></div>
        <h3>{title}</h3><p>{body}</p></article>)}</div>
    </section>
    <section className="cta shell">
      <div><Sparkles/><span>PRONTO PARA O FLOP?</span><h2>Sua próxima grande mão começa aqui.</h2></div>
      <Button variant="light" size="lg" onClick={() => startOAuthFlow('/lobby')}>Jogar agora <ArrowRight/></Button>
    </section>
    <footer className="footer shell">
      <div className="brand"><span className="brand-mark"><Club/></span><span>CTech <b>Poker</b></span></div>
      <div className="footer-content"><p>Jogue com responsabilidade. © {new Date().getFullYear()} A O CARVALHO TECH</p>
        <nav><a href="https://accounts.aoctech.app/products/poker" target="_blank" rel="noreferrer">Termos de Uso</a><a
          href="https://accounts.aoctech.app/products/poker-privacy" target="_blank" rel="noreferrer">Política de
          privacidade</a><a href="https://accounts.aoctech.app/legal" target="_blank" rel="noreferrer">Central
          Jurídica</a></nav>
      </div>
    </footer>
  </main>
}

function HeroTable() {
  return <div className="hero-visual" aria-label="Prévia de uma mesa de poker">
    <div className="ambient ambient-one"/>
    <div className="ambient ambient-two"/>
    <div className="poker-table">
      <div className="rail"/>
      <div className="felt"><span className="pot">POTE <b>2.450</b></span>
        <div className="community">{['heart-10', 'spade-jack', 'diamond-queen'].map((c, i) => <Image key={c}
                                                                                                     src={`/svgs/${c}.svg`}
                                                                                                     alt="" width={70}
                                                                                                     height={98}
                                                                                                     style={{'--i': i} as React.CSSProperties}/>)}<span
          className="card-placeholder"/><span className="card-placeholder"/></div>
        <div className="table-logo"><Club/> CTECH</div>
      </div>
      {[['Kely', '1.820', 'top'], ['Você', '3.240', 'bottom'], ['Wellington', '980', 'left'], ['Thiago', '2.100', 'right']].map(([name, chips, pos]) =>
        <div className={`demo-seat ${pos}`} key={name}><span
          className="avatar">{name[0]}</span><span><b>{name}</b><small>{chips}</small></span>{name === 'Você' &&
            <div className="hole"><Image src="/svgs/spade-ace.svg" alt="Ás de espadas" width={42} height={59}/><Image
                src="/svgs/heart-ace.svg" alt="Ás de copas" width={42} height={59}/></div>}</div>)}
      <div className="chip-orbit chip-a"/>
      <div className="chip-orbit chip-b"/>
    </div>
  </div>
}
