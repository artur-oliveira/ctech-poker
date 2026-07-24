'use client';
import Link from 'next/link';
import Image from 'next/image';
import {ChevronLeft, Compass} from 'lucide-react';

export default function Guide() {
  return <main className="app-page">
    <section className="guide shell">
      <Link href="/lobby"><ChevronLeft/> Lobby</Link>
      <header>
        <small>GUIA DA MESA</small>
        <Compass aria-hidden="true"/>
        <h1>Como funciona o CTech Poker</h1>
        <p>Do lobby até a primeira mão: onde escolher uma mesa, como entrar, como criar uma sala só para os seus
          amigos e como agir quando chegar a sua vez.</p>
      </header>

      <article className="guide-section">
        <div>
          <h2>O lobby</h2>
          <p>Todas as mesas públicas aparecem agrupadas por stake (o par de blinds). Cada cartão mostra quantas mesas
            daquele stake estão ativas agora e para quantos jogadores. Ali também ficam a recompensa diária e o
            ranking da comunidade.</p>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/lobby.png" alt="Lobby do CTech Poker com stakes agrupados por blinds" width={1280}
                 height={800}/>
        </figure>
      </article>

      <article className="guide-section reverse">
        <div>
          <h2>Entrar em uma mesa</h2>
          <ol className="rules-steps">
            <li><span>Toque no stake que quiser jogar — você entra em uma mesa aberta ou o sistema cria uma na
              hora.</span></li>
            <li><span>Escolha quantas fichas levar para a mesa (buy-in). Nada é debitado antes de você
              confirmar.</span></li>
            <li><span>Pronto — você está sentado e a próxima mão já entra na fila.</span></li>
          </ol>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/buyin.png" alt="Painel de compra de fichas antes de sentar na mesa" width={1280}
                 height={800}/>
        </figure>
      </article>

      <article className="guide-section">
        <div>
          <h2>Criar uma sala privada</h2>
          <ol className="rules-steps">
            <li><span>No lobby, toque em <b>Mesa privada</b>.</span></li>
            <li><span>Escolha o stake e o número de lugares (6 ou 9).</span></li>
            <li><span>Compartilhe o link de convite gerado — só quem recebe o link entra na sua sala.</span></li>
          </ol>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/create-room.png" alt="Diálogo de criação de mesa privada com stakes e lugares"
                 width={1280} height={800}/>
        </figure>
      </article>

      <article className="guide-section reverse">
        <div>
          <h2>Como agir na sua vez</h2>
          <p>Na sua vez, a borda dourada marca o seu assento e a barra de ações aparece na parte inferior — as
            opções disponíveis mudam conforme o estado da mão. <b>Fold</b> desiste, <b>Check</b> passa a vez sem
            apostar, <b>Pagar</b> cobre a aposta em aberto e <b>Aumentar</b> eleva o valor com atalhos de
            meio pote, pote cheio ou all-in (máx).</p>
          <p>Prefere o teclado? As letras nos botões — <b>F</b>, <b>C</b>, <b>P</b>, <b>R</b> — são os atalhos.
            Sua força de mão estimada aparece junto às suas cartas, e o botão <b>?</b> no cabeçalho abre o ranking
            de mãos sem sair do jogo.</p>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/table-preflop.png" alt="Mesa no pré-flop com a barra de ações e a força da mão do jogador"
                 width={1280} height={800}/>
        </figure>
      </article>

      <article className="guide-section">
        <div>
          <h2>Flop, turn e river</h2>
          <p>A cada rodada de apostas concluída, uma nova carta comunitária entra no centro da mesa: três no flop,
            mais uma no turn e a última no river. O pote e o rake ficam sempre visíveis ao lado das cartas, e cada
            assento mostra um anel de contagem regressiva quando é a vez de agir — perder o prazo passa a vez
            automaticamente.</p>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/table-flop.png" alt="Mesa no flop com três cartas comunitárias reveladas"
                 width={1280} height={800}/>
        </figure>
      </article>

      <article className="guide-section reverse">
        <div>
          <h2>Showdown e pote lateral</h2>
          <p>Se mais de um jogador chegar ao showdown, as cartas de cada um ficam visíveis e a melhor combinação de
            5 cartas leva o pote. Quando alguém vai all-in com menos fichas que os demais, a mesa divide o pote em
            pote principal e pote lateral — cada jogador só concorre ao que apostou.</p>
          <p>Quer entender as regras completas por trás de cada fase e o ranking de mãos? Veja o
            <Link href="/poker-rules"> guia de regras</Link>.</p>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/table-showdown.png" alt="Showdown com as cartas de todos os jogadores reveladas"
                 width={1280} height={800}/>
        </figure>
      </article>
    </section>
  </main>;
}
