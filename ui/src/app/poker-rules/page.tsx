'use client';
import Link from 'next/link';
import {Award, BookOpen, ChevronLeft, Club, History, ShieldCheck, Trophy} from 'lucide-react';
import {HandRankings} from '@/components/HandRankings';
import {useOptionalSession} from "@/lib/auth/session";
import {ProfileMenu} from "@/components/lobby/ProfileMenu";
import {Button} from "@/components/ui/button";

const SECTIONS = [
  {id: 'maos', label: 'Mãos'},
  {id: 'blinds', label: 'Blinds'},
  {id: 'stakes', label: 'Stakes'},
  {id: 'acoes', label: 'Ações'},
  {id: 'fases', label: 'Fases da mão'},
  {id: 'rake', label: 'Rake & Transparência'}
];

export default function PokerRules() {
  const {authed} = useOptionalSession();

  return (
    <main className="app-page">
      <nav className="app-nav shell">
        <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        {authed ? <div className="header-right">
          <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
          <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
          <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
          <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
          <ProfileMenu/>
        </div> : <Link href="/"><ChevronLeft/> Voltar</Link>}
      </nav>
      <section className="rules shell">
        <header>
          <small>REFERÊNCIA RÁPIDA</small>
          <BookOpen aria-hidden="true"/>
          <h1>Regras do Texas Hold&apos;em</h1>
          <p>O essencial para sentar em qualquer mesa com confiança, da hierarquia das mãos às regras de aposta e transparência do jogo.</p>
        </header>
        <nav className="rules-toc" aria-label="Seções desta página">
          {SECTIONS.map(s => <a key={s.id} href={`#${s.id}`}>{s.label}</a>)}
        </nav>

        <article id="maos" className="rules-section">
          <h2>Ranking de mãos</h2>
          <p>Da combinação mais forte à mais fraca. Em caso de empate na mesma categoria, vence quem tem as cartas mais altas (kicker) dentro dela.</p>
          <HandRankings/>
        </article>

        <article id="blinds" className="rules-section">
          <h2>Blinds</h2>
          <p>Antes de qualquer carta ser distribuída, os dois jogadores à esquerda do dealer postam uma aposta obrigatória: o <b>small blind</b> e, ao lado dele, o <b>big blind</b> (o dobro do valor). Isso garante que sempre haja fichas no pote para disputar em cada rodada.</p>
        </article>

        <article id="stakes" className="rules-section">
          <h2>Stakes e Buy-in</h2>
          <p>O par de blinds de uma mesa (por exemplo, 25 / 50) define o seu stake e a faixa de compra de fichas (buy-in mínimo e máximo), permitindo que você entre com um cacife adequado à sua banca sandbox.</p>
        </article>

        <article id="acoes" className="rules-section">
          <h2>Ações na sua vez</h2>
          <ul className="rules-list">
            <li><span><b>Fold</b>: desiste da mão e abre mão das fichas já apostadas nela.</span></li>
            <li><span><b>Check</b>: passa a vez sem apostar (somente quando não há aposta pendente na rodada).</span></li>
            <li><span><b>Pagar (Call)</b>: cobre a aposta em aberto para continuar na disputa do pote.</span></li>
            <li><span><b>Aumentar (Raise)</b>: eleva a aposta em aberto, exigindo que os adversários igualem para permanecer na mão.</span></li>
          </ul>
        </article>

        <article id="fases" className="rules-section">
          <h2>Fases de uma mão</h2>
          <ol className="rules-steps">
            <li><span><b>Pré-flop</b>: cada jogador recebe duas cartas fechadas. A rodada de apostas inicia no jogador à esquerda do big blind.</span></li>
            <li><span><b>Flop</b>: três cartas comunitárias são abertas no centro da mesa.</span></li>
            <li><span><b>Turn</b>: a quarta carta comunitária é revelada.</span></li>
            <li><span><b>River</b>: a quinta e última carta comunitária é revelada.</span></li>
            <li><span><b>Showdown</b>: quem permaneceu na mão revela suas cartas. A melhor combinação de 5 cartas (entre as 2 da mão e as 5 da mesa) vence o pote.</span></li>
          </ol>
        </article>

        <article id="rake" className="rules-section">
          <h2>Rake e Integridade Criptográfica</h2>
          <p>Nas mesas sandbox, uma pequena comissão (rake) é aplicada proporcionalmente sobre o pote acumulado para manter o equilíbrio econômico da partida. O valor retido é transparente e exibido ao lado do pote em tempo real.</p>
          <div className="rules-fairness-box">
            <ShieldCheck aria-hidden="true"/>
            <div>
              <strong>Embaralhamento Provably Fair (SHA-256)</strong>
              <p>Cada baralho é selado com um hash criptográfico antes de a mão começar. Ao final da jogada, a chave secreta (server seed) é revelada no histórico para que você possa verificar auditavelmente a ordem exata de cada carta.</p>
            </div>
          </div>
        </article>

        <div className="rules-footer-cta">
          <h3>Quer ver como tudo isso funciona na prática?</h3>
          <p>Confira nosso guia passo a passo da mesa ou entre direto no lobby para escolher um stake.</p>
          <div className="rules-cta-buttons">
            <Button render={<Link href="/guide"/>}>Ver Guia da Mesa</Button>
            <Button variant="outline" render={<Link href={authed ? "/lobby" : "/"}/>}>
              {authed ? "Ir para o Lobby" : "Ir para o Início"}
            </Button>
          </div>
        </div>
      </section>
    </main>
  );
}
