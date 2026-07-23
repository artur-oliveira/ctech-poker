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
          <h2>Como jogar</h2>
          <p>Na sua vez, a barra de ações na parte inferior mostra o que você pode fazer — as opções disponíveis
            mudam conforme o estado da mão. Use o botão <b>?</b> no cabeçalho da mesa para consultar o ranking de
            mãos sem sair do jogo.</p>
          <p>Quer entender as regras completas por trás de cada ação? Veja o <Link href="/poker-rules">guia de
            regras</Link>.</p>
        </div>
        <figure className="guide-shot">
          <Image src="/guide/table.png" alt="Mesa em andamento com a barra de ações disponível" width={1280}
            height={800}/>
        </figure>
      </article>
    </section>
  </main>;
}
