'use client';

import Link from 'next/link';
import {
  ArrowRight,
  Award,
  BookOpen,
  CircleDollarSign,
  Compass,
  History,
  ShieldCheck,
  ShoppingBag,
  Spade,
  Sparkles,
  Trophy,
  UserRound
} from 'lucide-react';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {Button} from '@/components/ui/button';
import {useOptionalSession} from '@/lib/auth/session';

const TOPICS = [
  {
    href: '/guide/basics', icon: Compass, title: 'Primeiros passos', time: '4 min',
    description: 'Blinds, tamanho da mesa, buy-in, auto rebuy e mesas privadas.',
    features: ['Lobby', 'Buy-in', 'Convites']
  },
  {
    href: '/guide/table', icon: Spade, title: 'Tudo sobre a mesa', time: '10 min',
    description: 'Controles, atalhos, time bank, ação preparada, espiar cartas, potes, rabbit hunt e preferências.',
    features: ['Apostas', 'Atalhos', 'Ferramentas']
  },
  {
    href: '/guide/hands', icon: History, title: 'Mãos, replay e integridade', time: '5 min',
    description: 'Histórico, detalhes, replay ação por ação, Provably Fair, exportação e links que expiram.',
    features: ['Replay', 'Prova', 'Compartilhar']
  },
  {
    href: '/guide/achievements', icon: Award, title: 'Conquistas', time: '4 min',
    description: 'Estrelas, maestria, próxima meta, conquistas secretas e destaques do perfil.',
    features: ['Metas', 'Estrelas', 'Progresso']
  },
  {
    href: '/guide/store', icon: ShoppingBag, title: 'Loja', time: '5 min',
    description: 'Reações premium, baralhos, feltros, recompensa diária, pacotes via Pix e estornos.',
    features: ['Premium', 'Pix', 'Estornos']
  },
  {
    href: '/guide/profile', icon: UserRound, title: 'Perfil e seu jogo', time: '5 min',
    description: 'Nome, foto, baralho, vitrine pública, presença e estatísticas privadas de estilo pré-flop.',
    features: ['Vitrine', 'HUD', 'Privacidade']
  },
  {
    href: '/guide/community', icon: Trophy, title: 'Comunidade e jogo seguro', time: '6 min',
    description: 'Ranking, amigos, convites, silenciar, bloquear, denunciar e pausa consciente.',
    features: ['Ranking', 'Pessoas', 'Segurança']
  }
];

export default function Guide() {
  const {authed} = useOptionalSession();
  return <AppPage authed={authed} current="guide">
    <AppPageBody className="guide guide-home">
      <AppPageHeader
        variant="feature"
        icon={BookOpen}
        eyebrow="CENTRAL DE AJUDA"
        title="Aprenda no seu ritmo"
        description="Comece uma partida em poucos minutos ou consulte uma função específica. Cada guia explica as funcionalidades do CTech poker."
      />

      <section className="guide-quickstart" aria-labelledby="quickstart-title">
        <div className="guide-quickstart-copy">
          <span><Sparkles aria-hidden="true"/> Para quem quer jogar agora</span>
          <h2 id="quickstart-title">
            Sua primeira mão em quatro passos
          </h2>
          <p>O CTech Poker usa fichas fictícias no modo sandbox.
            Escolha uma stake, uma mesa, confirme o buy-in e espere a próxima mão começar.
          </p>
          <Button render={<Link href="/guide/basics"/>}>Ver primeiros passos <ArrowRight aria-hidden="true"/></Button>
        </div>
        <ol className="guide-quickstart-steps">
          <li><b>Escolha</b><span>uma stake no lobby</span></li>
          <li><b>Selecione</b><span>uma mesa</span></li>
          <li><b>Confirme</b><span>quantas fichas levar</span></li>
          <li><b>Jogue</b><span>quando sua vez acender</span></li>
        </ol>
      </section>

      <div className="guide-directory-heading">
        <div>
          <h2>Explore por tópico</h2>
          <p>Do básico aos recursos avançados.</p>
        </div>
        <Link href="/poker-rules">
          <ShieldCheck aria-hidden="true"/> Regras do Texas Hold’em</Link>
      </div>
      <div className="guide-directory">
        {TOPICS.map(({href, icon: Icon, title, time, description, features}, index) => <Link key={href} href={href}
                                                                                             className={index === 1 ? 'featured' : undefined}>
          <span className="guide-directory-icon"><Icon aria-hidden="true"/></span>
          <span className="guide-directory-copy"><span><b>{title}</b><small>{time}</small></span><p>{description}</p>
            <span className="guide-directory-tags">{features.map(feature => <em key={feature}>{feature}</em>)}</span>
          </span>
          <ArrowRight className="guide-directory-arrow" aria-hidden="true"/>
        </Link>)}
      </div>

      <section className="guide-sandbox-note">
        <CircleDollarSign aria-hidden="true"/>
        <div><h2>Sobre fichas e dinheiro real</h2>
          <p>O ambiente disponível é sandbox: fichas servem apenas para jogar e não podem ser sacadas. A interface pode
            exibir a opção de carteira real, mas ela depende de liberação do serviço e nunca deve ser presumida como
            ativa.</p></div>
      </section>
    </AppPageBody>
  </AppPage>;
}
