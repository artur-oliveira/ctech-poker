'use client';

import {Spade} from 'lucide-react';
import {
  GuideBullets,
  GuideCallout,
  GuideLink,
  GuidePage,
  GuideSteps,
  GuideTerm,
  GuideTerms
} from '@/components/guide/GuidePage';

export default function TableGuide() {
  return <GuidePage icon={Spade} eyebrow="JOGO EM TEMPO REAL" title="Tudo o que acontece na mesa"
                    description="Controles de aposta, tempo de decisão, ferramentas sociais, resultados e situações especiais em uma única referência."
                    currentHref="/guide/table" next={{href: '/guide/hands', label: 'Rever mãos jogadas'}} sections={[
    {
      id: 'cabecalho',
      title: 'Cabeçalho e estado da conexão',
      summary: 'As ações globais ficam no topo; o feltro permanece dedicado à mão.',
      image: {src: '/guide/table-preflop.webp', alt: 'Mesa pré-flop com cabeçalho, assentos, cartas e barra de ações'},
      body: <><GuideTerms><GuideTerm term="Lobby">Volta à lista de mesas. Se você continuar sentado, poderá retornar
        pelo aviso de mesa ativa.</GuideTerm>
        <GuideTerm term="Ao vivo / Reconectando">Mostra o estado do socket. Durante uma queda, ações são bloqueadas até
          o estado ser sincronizado novamente.</GuideTerm>
        <GuideTerm term="Ranking de mãos">Abre a ordem das combinações sem tirar você da mesa.</GuideTerm>
        <GuideTerm term="Preferências">Tema do feltro, dealer auditivo, comandos por voz, rodar duas vezes quando
          permitido e lembrete de sessão.</GuideTerm>
        <GuideTerm term="Convidar">Copia ou compartilha o link da mesa quando a sala permite convites.</GuideTerm>
        <GuideTerm term="Sentar fora / voltar">Pausa ou retoma sua participação nas próximas mãos.</GuideTerm>
        <GuideTerm term="Sair">Confirma a saída e devolve a pilha restante à carteira usada no
          buy-in.</GuideTerm></GuideTerms></>
    },
    {
      id: 'controles',
      title: 'Ações na sua vez',
      summary: 'A barra habilita somente as decisões válidas para o estado atual.',
      body: <><GuideTerms><GuideTerm term="Fold (F)">Desiste da mão e perde apenas o que já colocou no pote.</GuideTerm>
        <GuideTerm term="Check (C)">Passa sem apostar quando ninguém aumentou o valor exigido.</GuideTerm>
        <GuideTerm term="Pagar (P)">Iguala o valor necessário para continuar.</GuideTerm>
        <GuideTerm term="Aumentar (R)">Escolhe um total entre o mínimo e o máximo. Atalhos incluem mínimo, ⅓, ½, ⅔ do
          pote, pote e máximo quando válidos.</GuideTerm>
        <GuideTerm term="All in (A)">Move toda a pilha efetiva disponível. O rótulo aparece quando o aumento selecionado
          alcança o máximo.</GuideTerm></GuideTerms>
        <p>No desktop, setas ajustam o valor; <b>H</b> seleciona meio pote e <b>A</b> leva ao máximo. No celular, toque
          primeiro em <b>Aumentar</b> para abrir o seletor compacto e confirme no segundo toque.</p>
        <GuideCallout kind="warning" title="Uma ação é definitiva">
          O botão mostra um estado de envio enquanto o servidor
          confirma. Não há desfazer depois que a ação é aceita.
        </GuideCallout></>
    },
    {
      id: 'tempo',
      title: 'Relógio, time bank e pré-ações',
      summary: 'Prepare uma resposta antes da sua vez e entenda o que ocorre quando o prazo acaba.',
      body: <>
        <p>A linha amarela no assento marca o tempo limite para a jogada. Depois deste tempo, o <b>time bank</b> entra
          automaticamente quando disponível; ele recupera 5 segundos a cada mão até o limite de 30 segundos.</p>
        <GuideBullets>
          <li><span><b>Check / Fold:</b> dá check se for gratuito; caso contrário, desiste.</span></li>
          <li><span><b>Fold:</b> desiste assim que a vez chegar.</span></li>
          <li><span><b>Call com valor:</b> paga somente o valor mostrado e cancela se a aposta subir.</span></li>
          <li><span><b>Call Any:</b> aceita qualquer valor legal quando chegar sua vez.</span></li>
        </GuideBullets>
        <p>Sem uma ação válida antes do tempo limite o sistema aplica a decisão segura permitida pelo estado,
          normalmente check quando gratuito ou fold quando há aposta.
        </p>
      </>
    },
    {
      id: 'fases',
      title: 'Pré-flop, flop, turn, river e showdown',
      summary: 'As cartas comunitárias e rodadas de aposta avançam em etapas visíveis.',
      image: {src: '/guide/table-flop.webp', alt: 'Mesa no flop com três cartas comunitárias, pote e ações'},
      body: <><GuideSteps>
        <li><span><b>Pré-flop:</b> cada jogador recebe duas cartas; small e big blind já estão no pote.</span></li>
        <li><span><b>Flop:</b> três cartas comunitárias são abertas.</span></li>
        <li><span><b>Turn:</b> a quarta carta comunitária é aberta.</span></li>
        <li><span><b>River:</b> a quinta carta completa o board.</span></li>
        <li><span><b>Showdown:</b> quem permaneceu compara a melhor combinação de cinco cartas.</span></li>
      </GuideSteps>
        <GuideCallout kind="info" title="Cartas escondidas permanecem escondidas">O histórico e a mesa mostram cartas de
          adversários somente quando elas foram realmente reveladas. O sistema não reconstrói informação
          privada.</GuideCallout></>
    },
    {
      id: 'potes',
      title: 'Pote principal, side pots, empate e rodar duas vezes',
      summary: 'All-ins podem dividir tanto o dinheiro apostado quanto o board.',
      body: <><GuideTerms><GuideTerm term="Pote principal">Valor pelo qual todos os jogadores elegíveis
        competem.</GuideTerm>
        <GuideTerm term="Pote lateral">Surge quando um all-in menor não cobre apostas posteriores. Cada pote lista seu
          próprio valor e só pode ser vencido por quem contribuiu para ele.</GuideTerm>
        <GuideTerm term="Empate">O pote é dividido entre mãos equivalentes; fichas indivisíveis seguem a regra da
          mesa.</GuideTerm>
        <GuideTerm term="Rodar duas vezes">Em um all-in, divide cada pote entre dois boards. Só acontece se a sala
          oferecer o recurso e todos os envolvidos o ativarem nas preferências.</GuideTerm></GuideTerms></>
    },
    {
      id: 'resultado',
      title: 'Vitórias, derrotas, fold e rake',
      summary: 'O resultado explica a combinação decisiva e a mudança na sua pilha.',
      image: {src: '/guide/table-showdown.webp', alt: 'Showdown com cartas reveladas e resultado da mão'},
      body: <><p>O painel de resultado diferencia vitória, derrota, empate, resultado misto entre potes e vitória porque
        todos desistiram. Quando houve showdown, ele destaca as cartas que formaram a combinação e pode explicar um
        desempate por kicker.</p>
        <GuideTerms><GuideTerm term="Rake">Comissão da mesa calculada sobre o pote. O valor aparece junto ao pote em
          tempo real e também integra o registro da mão.</GuideTerm>
          <GuideTerm term="Mostrar cartas">Quando permitido após o fim, você decide quais cartas próprias revelar. Não
            expõe automaticamente o que ficou oculto.</GuideTerm>
          <GuideTerm term="Rabbit hunt">Depois de um fold que encerra a mão, pode revelar quais cartas comunitárias
            viriam, usando a prova do baralho. É curiosidade pós-jogo e não altera o resultado.</GuideTerm></GuideTerms>
        <GuideLink href="/poker-rules#rake">Ler a regra de rake e transparência</GuideLink></>
    },
    {
      id: 'social',
      title: 'Chat, reações, notas e últimos vencedores',
      summary: 'Ferramentas sociais ficam nas bordas da mesa para não competir com a decisão.',
      body: <><GuideBullets>
        <li>
          <span><b>Chat:</b> envia mensagens para os participantes da mesa; fica indisponível durante reconexão.</span>
        </li>
        <li><span><b>Reações rápidas:</b> aplausos, risada, surpresa e outras respostas visuais. Objetos como café ou ficha pedem que você escolha um assento-alvo.</span>
        </li>
        <li><span><b>Silenciar reações:</b> interrompe animações recebidas sem afetar a partida.</span></li>
        <li><span><b>Nota privada:</b> abra o controle no assento de um adversário para registrar uma leitura pessoal; somente sua conta vê o texto.</span>
        </li>
        <li><span><b>Últimos vencedores:</b> resume resultados recentes daquela mesa.</span></li>
      </GuideBullets></>
    },
    {
      id: 'preferencias', title: 'Preferências, voz e pausas', summary: 'Personalizações ficam salvas neste navegador.',
      body: <><GuideBullets>
        <li><span><b>Tema:</b> Clássico, Meia-noite, Bordô ou Oceano.</span></li>
        <li><span><b>Dealer auditivo:</b> narra cartas e ações principais.</span></li>
        <li><span><b>Comandos por voz:</b> push-to-talk para Fold, Check, Pagar, Aumentar ou All In. O jogo recebe a ação reconhecida, não o áudio.</span>
        </li>
        <li><span><b>Pausa consciente:</b> lembrete opcional a cada 30, 60, 90 ou 120 minutos, com tempo, buy-in e resultado da sessão. Nunca abre durante sua vez.</span>
        </li>
        <li><span><b>Rebuy:</b> quando sua pilha chega a zero e você está pausado, compra mais fichas dentro dos limites para continuar.</span>
        </li>
      </GuideBullets></>
    }
  ]}/>;
}
