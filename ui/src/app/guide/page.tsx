'use client';
import Link from 'next/link';
import Image from 'next/image';
import {Award, BookOpen, ChevronLeft, Club, Compass, History, Keyboard, Trophy} from 'lucide-react';
import {useOptionalSession} from "@/lib/auth/session";
import {ProfileMenu} from "@/components/lobby/ProfileMenu";
import {Button} from "@/components/ui/button";

const SECTIONS = [
  {id: 'lobby', label: 'O lobby'},
  {id: 'entrar', label: 'Entrar na mesa'},
  {id: 'privada', label: 'Mesa privada'},
  {id: 'acoes', label: 'Ações na vez'},
  {id: 'atalhos', label: 'Atalhos & Dicas'},
  {id: 'fases', label: 'Flop, turn e river'},
  {id: 'showdown', label: 'Showdown & Replay'}
];

export default function Guide() {
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
      <section className="guide shell">
        {authed && <Link href="/lobby"><ChevronLeft/> Lobby</Link>}
        <header>
          <small>GUIA DA MESA</small>
          <Compass aria-hidden="true"/>
          <h1>Como funciona o CTech Poker</h1>
          <p>Do lobby até a vitória: onde escolher uma mesa, como criar salas privadas para amigos, usar atalhos de
            teclado e rever suas jogadas em replay.</p>
        </header>
        
        <nav className="rules-toc" aria-label="Seções do guia">
          {SECTIONS.map(s => <a key={s.id} href={`#${s.id}`}>{s.label}</a>)}
        </nav>
        
        <article id="lobby" className="guide-section">
          <div>
            <h2>O lobby de mesas</h2>
            <p>No lobby você encontra todas as mesas públicas organizadas por stake (par de blinds). Cada cartão indica
              o número de lugares (2 a 9 assentos) e quantas mesas daquele nível estão ativas no momento. É também no
              lobby que você resgata seu bônus diário de fichas e acessa o ranking da comunidade.</p>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/lobby.png" alt="Lobby do CTech Poker com stakes agrupados por blinds" width={1280}
                   height={800}/>
          </figure>
        </article>
        
        <article id="entrar" className="guide-section reverse">
          <div>
            <h2>Entrar em uma mesa</h2>
            <ol className="rules-steps">
              <li><span>Selecione o stake que deseja jogar no lobby. O sistema conecta você a uma mesa existente ou cria uma nova instantaneamente.</span>
              </li>
              <li><span>Defina o valor do seu buy-in (quantidade de fichas para entrar). Nada é debitado antes da sua confirmação.</span>
              </li>
              <li><span>Você ocupa seu assento e entra na rodada assim que a próxima mão for iniciada.</span></li>
            </ol>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/buyin.png" alt="Painel de compra de fichas antes de sentar na mesa" width={1280}
                   height={800}/>
          </figure>
        </article>
        
        <article id="privada" className="guide-section">
          <div>
            <h2>Criar uma sala privada</h2>
            <ol className="rules-steps">
              <li><span>No lobby, clique em <b>Mesa privada</b>.</span></li>
              <li><span>Escolha o stake desejado e a quantidade de assentos (6 ou 9 lugares).</span></li>
              <li>
                <span>Copie e compartilhe o link de convite exclusivo. Somente quem tem o link pode acessar sua sala.</span>
              </li>
            </ol>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/create-room.png" alt="Diálogo de criação de mesa privada com stakes e lugares"
                   width={1280} height={800}/>
          </figure>
        </article>
        
        <article id="acoes" className="guide-section reverse">
          <div>
            <h2>Como agir na sua vez</h2>
            <p>Na sua vez de jogar, uma borda iluminada destaca seu assento e a barra de ações é ativada. As opções se
              adaptam ao momento da mão: <b>Fold</b> (desistir), <b>Check</b> (passar a vez), <b>Pagar</b> (igualar
              aposta) e <b>Aumentar</b> (elevar a aposta com atalhos de meio pote, pote cheio ou all-in).</p>
            <p>O tempo de ação é contado por um anel circular no assento. Caso o tempo expire sem interação, a mão é
              passada ou descartada automaticamente para manter a partida fluida.</p>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/table-preflop.png"
                   alt="Mesa no pré-flop com a barra de ações e a força da mão do jogador"
                   width={1280} height={800}/>
          </figure>
        </article>
        
        <article id="atalhos" className="guide-section">
          <div>
            <h2><Keyboard aria-hidden="true"/> Atalhos de teclado & Dicas</h2>
            <p>Jogue mais rápido usando os atalhos de teclado nativos durante a partida:</p>
            <ul className="rules-list">
              <li><span><b>F</b>: Desistir (Fold)</span></li>
              <li><span><b>C</b>: Passar a vez (Check) ou Pagar (Call)</span></li>
              <li><span><b>P</b>: Aumentar para Pote Cheiro (Pot Raise)</span></li>
              <li><span><b>A</b>: All in</span></li>
              <li><span><b>R</b>: Aumentar (Raise)</span></li>
            </ul>
            <p>A força estimada da sua mão é exibida abaixo das suas cartas, e o botão <b>?</b> no topo abre o ranking
              completo de combinações sem sair do jogo.</p>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/table-flop.png" alt="Mesa no flop mostrando indicadores e controles"
                   width={1280} height={800}/>
          </figure>
        </article>
        
        <article id="fases" className="guide-section reverse">
          <div>
            <h2>Flop, turn e river</h2>
            <p>Conforme as rodadas de apostas avançam, as cartas comunitárias surgem no centro do feltro: 3 no flop, 1
              no turn e 1 no river. O pote principal, potes laterais e o rake em tempo real ficam destacados no centro
              da mesa.</p>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/table-flop.png" alt="Mesa no flop com três cartas comunitárias reveladas"
                   width={1280} height={800}/>
          </figure>
        </article>
        
        <article id="showdown" className="guide-section">
          <div>
            <h2>Showdown, Replay & Provably Fair</h2>
            <p>No showdown, os jogadores remanescentes revelam suas cartas e o sistema premia a melhor combinação de 5
              cartas. Todas as mãos jogadas ficam salvas na sua aba <b>Mãos</b>.</p>
            <p>Em cada mão salva você pode abrir o <b>Replayer de Mãos Interativo</b> para rever jogada por jogada ou
              conferir o código SHA-256 de verificação <b>Provably Fair</b>.</p>
          </div>
          <figure className="guide-shot">
            <Image src="/guide/table-showdown.png" alt="Showdown com as cartas de todos os jogadores reveladas"
                   width={1280} height={800}/>
          </figure>
        </article>
        
        <div className="guide-footer-cta">
          <h3>Pronto para testar na prática?</h3>
          <p>Entre em uma mesa sandbox com fichas fictícias e experimente o CTech Poker agora mesmo.</p>
          <div className="rules-cta-buttons">
            <Button render={<Link href={authed ? "/lobby" : "/"}/>}>
              {authed ? "Ir para o Lobby" : "Começar a Jogar"}
            </Button>
            <Button variant="outline" render={<Link href="/poker-rules"/>}>
              Guia de Regras
            </Button>
          </div>
        </div>
      </section>
    </main>
  );
}
