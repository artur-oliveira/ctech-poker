'use client';
import Link from 'next/link';
import {BookOpen, ChevronLeft} from 'lucide-react';
import {HandRankings} from '@/components/HandRankings';

const SECTIONS = [
  {id: 'maos', label: 'Mãos'},
  {id: 'blinds', label: 'Blinds'},
  {id: 'stakes', label: 'Stakes'},
  {id: 'acoes', label: 'Ações'},
  {id: 'fases', label: 'Fases da mão'},
  {id: 'rake', label: 'Rake'}
];

export default function PokerRules() {
  return <main className="app-page">
    <section className="rules shell">
      <Link href="/lobby"><ChevronLeft/> Lobby</Link>
      <header>
        <small>REFERÊNCIA RÁPIDA</small>
        <BookOpen aria-hidden="true"/>
        <h1>Regras do Texas Hold&apos;em</h1>
        <p>O essencial para sentar em qualquer mesa com confiança — da força das mãos ao motivo de a casa cobrar
          rake.</p>
      </header>
      <nav className="rules-toc" aria-label="Seções desta página">
        {SECTIONS.map(s => <a key={s.id} href={`#${s.id}`}>{s.label}</a>)}
      </nav>

      <article id="maos" className="rules-section">
        <h2>Ranking de mãos</h2>
        <p>Da mais forte à mais fraca. Em caso de empate na mesma categoria, vence quem tem as cartas mais altas
          dentro dela.</p>
        <HandRankings/>
      </article>

      <article id="blinds" className="rules-section">
        <h2>Blinds</h2>
        <p>Antes de qualquer carta ser distribuída, os dois jogadores à esquerda do dealer postam uma aposta
          obrigatória: o <b>small blind</b> e, ao lado dele, o <b>big blind</b> — o dobro do valor. Isso garante que
          sempre haja algo no pote para disputar, mesmo que todos desistam ainda na primeira rodada.</p>
      </article>

      <article id="stakes" className="rules-section">
        <h2>Stakes</h2>
        <p>O par de blinds de uma mesa (por exemplo, 25 / 50) define o seu stake e, junto dele, a faixa de compra de
          fichas — normalmente entre 20 e 100 vezes o big blind. Toda mesa aqui no CTech Poker usa fichas virtuais do
          sandbox: nada de dinheiro real entra ou sai do jogo.</p>
      </article>

      <article id="acoes" className="rules-section">
        <h2>Ações na sua vez</h2>
        <ul className="rules-list">
          <li><span><b>Fold</b> — desiste da mão e perde o que já apostou nela.</span></li>
          <li><span><b>Check</b> — passa a vez sem apostar; só é possível quando nenhuma aposta está em aberto na
            rodada.</span></li>
          <li><span><b>Pagar</b> — cobre a maior aposta em aberto para continuar na mão.</span></li>
          <li><span><b>Aumentar</b> — eleva a aposta em aberto, forçando os outros a pagar mais para continuar.</span>
          </li>
        </ul>
      </article>

      <article id="fases" className="rules-section">
        <h2>Fases da mão</h2>
        <ol className="rules-steps">
          <li><span><b>Pré-flop</b> — cada jogador recebe duas cartas fechadas; a primeira rodada de apostas começa
            pelo jogador seguinte ao big blind.</span></li>
          <li><span><b>Flop</b> — três cartas comunitárias são reveladas no centro da mesa.</span></li>
          <li><span><b>Turn</b> — uma quarta carta comunitária é revelada.</span></li>
          <li><span><b>River</b> — a quinta e última carta comunitária é revelada.</span></li>
          <li><span><b>Showdown</b> — quem ainda está na mão mostra as cartas; a melhor combinação de 5 cartas (entre
            as 2 na mão e as 5 da mesa) leva o pote.</span></li>
        </ol>
      </article>

      <article id="rake" className="rules-section">
        <h2>Rake</h2>
        <p>O rake é a comissão que a casa retém sobre o pote de cada mão — é assim que a mesa se sustenta. O valor
          fica sempre visível ao lado do pote durante o jogo, nunca embutido ou escondido.</p>
      </article>
    </section>
  </main>;
}
