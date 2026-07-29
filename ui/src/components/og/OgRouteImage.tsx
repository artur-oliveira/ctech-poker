import Image from 'next/image';
import {Award, BookOpen, Club, Crown, History, ShieldCheck, Trophy, Users} from 'lucide-react';
import {cardLabel, cardPath} from '@/lib/cards';

export type OgSlug = typeof OG_IMAGE_DATA[number]['slug'];

export const OG_IMAGE_DATA = [
  {
    slug: 'home',
    title: 'Sua mesa de poker, sempre pronta.',
    subtitle: 'Texas Hold’em social e auditável no navegador.'
  },
  {slug: 'guide', title: 'Como funciona o CTech Poker', subtitle: 'Da escolha da mesa ao showdown.'},
  {slug: 'poker-rules', title: 'Regras do Texas Hold’em', subtitle: 'Rodadas, ações e ordem das mãos sem mistério.'},
  {slug: 'profile', title: 'Ana', subtitle: 'Vitrine pública do jogador'},
  {slug: 'lobby', title: 'Escolha sua mesa.', subtitle: 'Fichas virtuais, emoção de verdade.'},
  {slug: 'table', title: 'Mesa 25 / 50', subtitle: 'Sua vez · 18 segundos'},
  {slug: 'hands', title: 'Mãos jogadas', subtitle: 'Resultados e auditoria de cada partida.'},
  {slug: 'hand-history', title: 'Detalhes da Mão', subtitle: '12 min atrás · Mesa 01ARZ3ND…'},
  {slug: 'hand-replay', title: 'Replay da mão', subtitle: 'Ação 5 de 8 · Flop'},
  {slug: 'shared-hand', title: '+4.200 fichas', subtitle: 'Mão compartilhada · jogadores anonimizados'},
  {slug: 'leaderboard', title: 'Ranking da comunidade', subtitle: 'Desempenho nas mesas sandbox.'},
  {slug: 'achievements', title: 'Conquistas', subtitle: 'Cada estrela representa uma meta vencida.'}
] as const;

const cards = ['9c', '5c', '6h', '8h', '3h'];

function Brand() {
  return <div className="og-real-brand"><span><Club/></span> CTech <b>Poker</b></div>;
}

function CardRow({hole = false}: { hole?: boolean }) {
  const shown = hole ? ['Kd', 'Kc'] : cards;
  return <div className="og-real-cards" aria-label={hole ? 'Reis de ouros e paus' : 'Board da mão'}>
    {shown.map(card => <Image key={card} src={cardPath(card)} alt={cardLabel(card)} width={58} height={78}/>)}
  </div>;
}

function OgChipStack({pot = false}: { pot?: boolean }) {
  return <span className={`og-chip-stack${pot ? ' pot' : ''}`} aria-hidden="true">
    <i/><i/><i/>
  </span>;
}

function TableScene() {
  const seats = [
    {position: 'top-left', initial: 'B', name: 'Bia', stack: '3.200', role: 'D', bet: '150'},
    {position: 'top', initial: 'L', name: 'Leo', stack: '2.850', role: 'SB', bet: '25'},
    {position: 'top-right', initial: 'N', name: 'Nina', stack: '5.100', role: 'BB', bet: '50'},
    {position: 'right', initial: 'R', name: 'Rafa', stack: '1.975', bet: '400'},
    {position: 'bottom-right', initial: 'C', name: 'Caio', stack: '4.300'},
    {position: 'bottom-left', initial: 'M', name: 'Maya', stack: '2.600', bet: '400'},
    {position: 'left', initial: 'G', name: 'Gui', stack: '3.750'}
  ];
  return <div className="og-table-game" aria-label="Mesa de poker sandbox em uma rodada no flop">
    <div className="og-table-rail"/>
    <div className="og-table-felt">
      <div className="og-table-board">
        <div className="og-table-pot"><OgChipStack pot/> POTE <b>1.450</b></div>
        <div className="og-table-community">
          {['9c', '5c', '6h'].map(card =>
            <Image key={card} src={cardPath(card)} alt={cardLabel(card)} width={52} height={73}/>)}
          <span/><span/>
        </div>
      </div>
    </div>
    {seats.map(seat => <div className={`og-table-seat ${seat.position}`} key={seat.name}>
      {seat.role && <em className={seat.role === 'D' ? 'dealer' : ''}>{seat.role}</em>}
      <div className="og-table-hole">
        <Image src={cardPath('back')} alt="Carta fechada" width={28} height={39}/>
        <Image src={cardPath('back')} alt="Carta fechada" width={28} height={39}/>
      </div>
      <i className="og-table-avatar">{seat.initial}</i>
      <span><b>{seat.name}</b><small>{seat.stack} fichas</small></span>
      {seat.bet && <strong><OgChipStack/> {seat.bet}</strong>}
    </div>)}
    <div className="og-table-seat viewer is-turn">
      <div className="og-table-hole">
        <Image src={cardPath('As')} alt={cardLabel('As')} width={34} height={48}/>
        <Image src={cardPath('Ah')} alt={cardLabel('Ah')} width={34} height={48}/>
      </div>
      <i className="og-table-avatar">V</i>
      <span><b>Você</b><small>4.850 fichas</small></span>
      <strong><OgChipStack/> 400</strong>
    </div>
    <div className="og-table-actions">
      <small>SUA VEZ · 18 SEGUNDOS</small>
      <div><span>FOLD <kbd>F</kbd></span><span>CHECK <kbd>C</kbd></span><b>PAGAR 400 <kbd>P</kbd></b></div>
    </div>
  </div>;
}

function LandingTableScene() {
  return <div className="og-landing-stage" aria-label="Prévia da mesa da página inicial">
    <div className="og-landing-table">
      <div className="og-landing-felt">
        <div className="landing-pot">POTE <b>2.450</b></div>
        <div className="landing-community">
          {['Th', 'Js', 'Qd'].map(card =>
            <Image key={card} src={cardPath(card)} alt={cardLabel(card)} width={54} height={76}/>)}
          <i/><i/>
        </div>
        <div className="landing-table-logo"><Club/> CTECH</div>
      </div>
      <div className="landing-seat top"><i>K</i><span><b>Kely</b><small>1.820</small></span></div>
      <div className="landing-seat left"><i>W</i><span><b>Wellington</b><small>980</small></span></div>
      <div className="landing-seat right"><i>T</i><span><b>Thiago</b><small>2.100</small></span></div>
      <div className="landing-seat bottom"><i>V</i><span><b>Você</b><small>3.240</small></span>
        <div className="landing-hole">
          <Image src={cardPath('As')} alt={cardLabel('As')} width={31} height={44}/>
          <Image src={cardPath('Ah')} alt={cardLabel('Ah')} width={31} height={44}/>
        </div>
      </div>
      <div className="landing-chip one">♛</div>
      <div className="landing-chip two">♛</div>
    </div>
  </div>;
}

function LobbyScene() {
  return <div className="og-real-list stakes">
    <div><small>SANDBOX · 6-MAX</small><b>25 / 50</b><span>6 de 6 jogadores</span></div>
    <div><small>SANDBOX · FULL-RING</small><b>50 / 100</b><span>7 de 9 jogadores</span></div>
    <div><small>SANDBOX · HEADS-UP</small><b>10 / 20</b><span>1 de 2 jogadores</span></div>
  </div>;
}

function RankingScene() {
  return <div className="og-real-list ranking">
    {[['1', 'Bia', '71 vitórias'], ['2', 'Leo', '52 vitórias'], ['3', 'Ana', '49 vitórias']].map(row =>
      <div key={row[1]}><i>{row[0]}</i><b>{row[1]}</b><span>{row[2]}</span></div>)}
  </div>;
}

function AchievementScene() {
  return <div className="og-real-achievements">
    <div><Trophy/><b>22</b><span>estrelas</span></div>
    <div><Award/><b>9</b><span>desbloqueadas</span></div>
    <div><Crown/><b>1</b><span>completa</span></div>
  </div>;
}

function HandScene({replay = false, shared = false}: { replay?: boolean; shared?: boolean }) {
  return <div className="og-real-hand">
    <div className="hero-hand"><small>{shared ? 'HERÓI' : 'VOCÊ'}</small><CardRow hole/><b>Par de Reis</b></div>
    <div className="hand-board"><small>BOARD COMUNITÁRIO</small><CardRow/></div>
    <strong>+4.200 fichas</strong>
    {replay && <div className="replay-timeline"><span/><span/><span className="active"/><span/><span/></div>}
  </div>;
}

function GuideScene({rules = false}: { rules?: boolean }) {
  return <div className="og-real-steps">
    {(rules ? [['1', 'Pré-flop'], ['2', 'Flop'], ['3', 'Turn'], ['4', 'River']] :
      [['1', 'Escolha a mesa'], ['2', 'Confirme o buy-in'], ['3', 'Jogue sua mão']]).map(step =>
      <div key={step[0]}><i>{step[0]}</i><b>{step[1]}</b></div>)}
  </div>;
}

function ProfileScene() {
  return <div className="og-real-profile">
    <div className="avatar">A</div>
    <div><small>ESTILO DE JOGO</small><b>Iniciativa alta</b><span>184 mãos · 49 vitórias</span></div>
    <div className="profile-win"><CardRow hole/><strong>+4.200</strong></div>
  </div>;
}

function sceneFor(slug: OgSlug) {
  if (slug === 'home') return <LandingTableScene/>;
  if (slug === 'table') return <TableScene/>;
  if (slug === 'lobby') return <LobbyScene/>;
  if (slug === 'leaderboard') return <RankingScene/>;
  if (slug === 'achievements') return <AchievementScene/>;
  if (slug === 'hands' || slug === 'hand-history') return <HandScene/>;
  if (slug === 'hand-replay') return <HandScene replay/>;
  if (slug === 'shared-hand') return <HandScene shared/>;
  if (slug === 'guide') return <GuideScene/>;
  if (slug === 'poker-rules') return <GuideScene rules/>;
  return <ProfileScene/>;
}

const icons = {
  home: Club, guide: BookOpen, 'poker-rules': ShieldCheck, profile: Users, lobby: Users, table: Club,
  hands: History, 'hand-history': ShieldCheck, 'hand-replay': History, 'shared-hand': Club,
  leaderboard: Trophy, achievements: Award
};

export function OgRouteImage({slug}: { slug: OgSlug }) {
  const data = OG_IMAGE_DATA.find(item => item.slug === slug)!;
  const Icon = icons[slug];
  return <article className={`og-route-image og-route-${slug}`}>
    <Brand/>
    <section className="og-real-copy">
      <span className="og-real-icon"><Icon/></span>
      <h1>{data.title}</h1>
      <p>{data.subtitle}</p>
      <small>{['lobby', 'table', 'hands', 'hand-history', 'hand-replay', 'leaderboard', 'achievements'].includes(slug)
        ? 'CONTEÚDO ILUSTRATIVO · AMBIENTE SANDBOX' : 'CTECH POKER · PROVABLY FAIR'}</small>
    </section>
    <section className="og-real-scene">{sceneFor(slug)}</section>
  </article>;
}
