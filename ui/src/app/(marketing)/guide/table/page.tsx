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
                    description="Controles, atalhos, tempo de decisão, cartas, resultados e ferramentas da mesa em uma referência só."
                    currentHref="/guide/table" next={{href: '/guide/hands', label: 'Rever mãos jogadas'}} sections={[
    {
      id: 'cabecalho',
      title: 'O cabeçalho da mesa',
      summary: 'Tudo o que não é a mão fica no topo; o feltro é só do jogo.',
      image: {src: '/guide/table-preflop.webp', alt: 'Mesa no pré-flop com cabeçalho, assentos, cartas e barra de ações'},
      body: <><GuideTerms>
        <GuideTerm term="Lobby">Volta à lista de mesas sem soltar seu assento. O aviso de mesa aberta traz você de
          volta.</GuideTerm>
        <GuideTerm term="Ao vivo / Reconectando">Estado da conexão. Durante uma reconexão, ações e chat ficam
          bloqueados até o estado da mesa chegar completo.</GuideTerm>
        <GuideTerm term="Maior pote de hoje">O maior pote já disputado nesta mesa hoje, com a mão vencedora quando
          ela foi revelada. Atualiza a cada mão concluída.</GuideTerm>
        <GuideTerm term="Mais ações da mesa">Reúne Ranking de mãos, Últimos vencedores e Preferências, mais Treinador e
          Convidar quando a mesa oferece os dois. Em telas largas esses itens também aparecem soltos no
          cabeçalho.</GuideTerm>
        <GuideTerm term="Sentar fora / Voltar a jogar">Pausa ou retoma sua participação a partir da próxima
          mão.</GuideTerm>
        <GuideTerm term="Sair da mesa">Pede a saída e devolve a pilha à carteira usada no buy-in.</GuideTerm>
      </GuideTerms>
        <p>Reações e chat têm botão próprio ao lado do menu; em telas largas os dois painéis ficam ancorados nas
          bordas do feltro. Com mouse, passar por cima do botão já abre o painel e o clique o mantém aberto;
          clicar de novo fecha.</p></>
    },
    {
      id: 'acoes',
      title: 'Ações na sua vez',
      summary: 'A barra habilita apenas o que é legal no estado atual.',
      body: <><GuideTerms>
        <GuideTerm term="Fold">Desiste da mão. Você perde o que já colocou no pote.</GuideTerm>
        <GuideTerm term="Check">Passa a vez sem apostar, quando ninguém aumentou.</GuideTerm>
        <GuideTerm term="Pagar">Iguala o valor exigido. O botão mostra quanto sai da sua pilha.</GuideTerm>
        <GuideTerm term="Aumentar">Define um total entre o mínimo e o máximo. Os atalhos de tamanho são Mín, ⅓, ½ e ⅔
          do pote, Pote e Máx, além do controle deslizante e dos botões de mais e menos.</GuideTerm>
        <GuideTerm term="All In">O botão troca de rótulo sozinho quando o valor escolhido alcança o
          máximo.</GuideTerm>
      </GuideTerms>
        <p>O número em <b>Aumentar</b> é o total da sua aposta, não o que você acrescenta — diferente de
          <b> Pagar</b>, que soma. A linha acima da barra mostra o stack efetivo: o máximo que ainda pode ser
          disputado contra quem continua na mão.</p>
        <p>No celular, o primeiro toque em <b>Aumentar</b> abre o seletor de valor e o segundo confirma. Segurar mais
          ou menos acelera o ajuste.</p>
        <GuideCallout kind="warning" title="Uma ação é definitiva">O botão fica em estado de envio enquanto o servidor
          confirma. Não há desfazer depois do aceite. Se a ação for recusada, a barra explica o motivo e devolve as
          escolhas ainda válidas.</GuideCallout></>
    },
    {
      id: 'atalhos',
      title: 'Atalhos do teclado',
      summary: 'As mesmas letras agem na sua vez e preparam a jogada fora dela.',
      body: <><GuideTerms variant="keys">
        <GuideTerm term="F">Fold. Fora da sua vez, prepara um fold.</GuideTerm>
        <GuideTerm term="C">Check. Fora da sua vez, prepara pagar o valor mostrado e, no toque seguinte, Call
          Any.</GuideTerm>
        <GuideTerm term="P">Pagar.</GuideTerm>
        <GuideTerm term="R">Confirma o aumento com o valor selecionado.</GuideTerm>
        <GuideTerm term="H">Leva o valor do aumento a meio pote.</GuideTerm>
        <GuideTerm term="A">Leva o valor ao máximo. Fora da sua vez, prepara um all-in.</GuideTerm>
        <GuideTerm term="X">Prepara Check / Fold.</GuideTerm>
        <GuideTerm term="1 / 2">Espia a sua primeira ou a sua segunda carta.</GuideTerm>
        <GuideTerm term="E">Abre e fecha as reações.</GuideTerm>
        <GuideTerm term="T">Abre e fecha o chat.</GuideTerm>
        <GuideTerm term="Setas">Ajustam o valor do aumento; com Ctrl, o passo é o triplo.</GuideTerm>
      </GuideTerms>
        <p>Nenhum atalho dispara enquanto você está digitando no chat.</p></>
    },
    {
      id: 'tempo',
      title: 'Relógio, time bank e ação preparada',
      summary: 'Decida antes da sua vez e saiba o que acontece quando o prazo acaba.',
      body: <>
        <p>O contorno do assento marca o tempo da decisão. Quando ele termina, entra o <b>time bank</b>: uma reserva
          pessoal que recupera 5 segundos por mão até o limite de 30. Enquanto ela é consumida, uma ampulheta aparece
          no assento — a mesa inteira vê que o jogador ainda está decidindo, não que caiu.</p>
        <p>Sem nenhuma ação até o fim do prazo, o sistema aplica a decisão segura daquele estado: check quando é
          grátis, fold quando há aposta.</p>
        <GuideBullets>
          <li><span><b>Check / Fold:</b> dá check se for grátis; caso contrário, desiste.</span></li>
          <li><span><b>Fold:</b> desiste assim que a vez chegar.</span></li>
          <li><span><b>Call com valor:</b> paga somente aquele valor e se cancela se a aposta subir.</span></li>
          <li><span><b>Call Any:</b> aceita qualquer valor legal quando chegar sua vez.</span></li>
          <li><span><b>All In:</b> vai com tudo na sua vez.</span></li>
        </GuideBullets>
        <p>A ação preparada dispara sozinha e a barra avisa que está executando. Tocar de novo na mesma opção cancela
          a preparação.</p>
      </>
    },
    {
      id: 'cartas',
      title: 'Suas cartas e o que o assento conta',
      summary: 'As cartas chegam viradas para baixo; você decide quando olhar.',
      body: <><GuideTerms>
        <GuideTerm term="Espiar">Toque na carta, ou use as teclas 1 e 2, para ver e esconder de novo. É uma ação sua: nenhum outro jogador
          vê que você olhou.</GuideTerm>
        <GuideTerm term="Chance e combinação">A estimativa de vitória e o nome da sua mão só aparecem depois que você
          espia as duas cartas, ou quando a mão termina.</GuideTerm>
        <GuideTerm term="Mostrar cartas">Terminada a mão, você escolhe quais das suas cartas revelar para a mesa —
          uma, as duas ou nenhuma.</GuideTerm>
        <GuideTerm term="Sequência">Um selo V ou D no assento conta as vitórias ou derrotas seguidas daquele
          jogador.</GuideTerm>
        <GuideTerm term="Estilo de jogo">Rótulo de tendência de quem tornou o próprio estilo público. É leitura, não
          garantia.</GuideTerm>
      </GuideTerms>
        <GuideCallout kind="safe" title="Carta escondida continua escondida">A mesa mostra apenas o que foi realmente
          revelado. O aplicativo nunca reconstrói uma carta que não recebeu — nem aqui, nem no histórico, nem no
          replay.</GuideCallout></>
    },
    {
      id: 'fases',
      title: 'Pré-flop, flop, turn, river e showdown',
      summary: 'A trilha embaixo do board mostra em que rua a mão está.',
      image: {src: '/guide/table-flop.webp', alt: 'Mesa no flop com três cartas comunitárias, pote e trilha de ruas'},
      body: <><GuideSteps>
        <li><span><b>Pré-flop:</b> cada jogador recebe duas cartas; small e big blind já estão no pote.</span></li>
        <li><span><b>Flop:</b> três cartas comunitárias são abertas.</span></li>
        <li><span><b>Turn:</b> a quarta carta comunitária.</span></li>
        <li><span><b>River:</b> a quinta completa o board.</span></li>
        <li><span><b>Showdown:</b> quem ficou compara a melhor combinação de cinco cartas.</span></li>
      </GuideSteps>
        <p>O dealer anuncia cada evento em uma faixa curta sobre o feltro; com o dealer auditivo ligado, o mesmo texto
          é falado.</p></>
    },
    {
      id: 'potes',
      title: 'Potes, empate e rodar duas vezes',
      summary: 'Um all-in pode dividir tanto as fichas quanto o board.',
      body: <><GuideTerms>
        <GuideTerm term="Pote principal">Valor disputado por todos os jogadores elegíveis.</GuideTerm>
        <GuideTerm term="Pote lateral">Nasce quando um all-in menor não cobre as apostas seguintes. Cada pote tem seu
          valor e só pode ser ganho por quem contribuiu para ele.</GuideTerm>
        <GuideTerm term="Empate">O pote é dividido entre mãos equivalentes. Naipe não desempata.</GuideTerm>
        <GuideTerm term="Devolução">Aposta que ninguém cobriu volta para quem apostou, antes do acerto.</GuideTerm>
        <GuideTerm term="Rodar duas vezes">Em um all-in, divide cada pote entre dois boards. Depende de a sala
          permitir e de todos os envolvidos terem ativado nas preferências.</GuideTerm>
        <GuideTerm term="Rake">Comissão da mesa sobre o pote. Aparece junto ao pote em tempo real e fica registrada na
          mão.</GuideTerm>
      </GuideTerms>
        <GuideLink href="/poker-rules#rake">Ler a regra de rake e integridade</GuideLink></>
    },
    {
      id: 'resultado',
      title: 'O resultado da mão',
      summary: 'O painel explica a combinação decisiva, cada pote e o que mudou na sua pilha.',
      image: {src: '/guide/table-showdown.webp', alt: 'Showdown com cartas reveladas, rake no pote e o painel de resultado da mão'},
      body: <><p>Vitória, derrota, empate, resultado misto entre potes e vitória por desistência têm painéis
        diferentes. Quando houve showdown, o painel destaca as cinco cartas que decidiram e diz quando o kicker foi o
        critério. Com mais de um pote, o acerto lista cada um, quem levou e quanto veio para você.</p>
        <GuideTerms>
          <GuideTerm term="Rabbit hunt">Depois de uma mão encerrada sem showdown, você pode pagar um big blind para
            ver as cartas comunitárias que viriam. O navegador verifica o baralho antes de mostrar; se a verificação
            falhar, a taxa volta. Não altera o resultado.</GuideTerm>
          <GuideTerm term="Pedir a mão do vencedor">Custa um big blind e é um pedido, não uma compra: o vencedor
            decide se mostra e fica com metade do valor. Se ele recusar ou não responder no prazo, suas fichas
            voltam.</GuideTerm>
        </GuideTerms>
        <p>Enquanto o resultado está na tela, um contador mostra quanto falta para a próxima distribuição.</p></>
    },
    {
      id: 'social',
      title: 'Reações, chat, notas e últimos vencedores',
      summary: 'As ferramentas sociais ficam nas bordas para não competir com a decisão.',
      image: {src: '/guide/table-reactions.webp', alt: 'Painel de reações aberto com favoritas, gestos na cadeira e gestos para outro jogador'},
      body: <><GuideBullets>
        <li><span><b>Reações:</b> 30 gestos, divididos entre os que nascem na sua cadeira e os que você manda para
          alguém. Nesses, escolha o gesto e depois toque no assento que vai recebê-lo. Seis são premium e ficam
          bloqueados até serem liberados na Loja.</span></li>
        <li><span><b>Favoritas:</b> até três reações ficam como atalho fixo ao lado do botão, sem abrir o
          painel.</span></li>
        <li><span><b>Intervalo:</b> há uma pausa curta entre um envio e o próximo, para ninguém cobrir a mesa de
          efeitos.</span></li>
        <li><span><b>Silenciar efeitos:</b> esconde as animações recebidas. Não afeta a partida e ninguém é
          avisado.</span></li>
        <li><span><b>Chat:</b> mensagens para a mesa, que também aparecem como balão no assento de quem falou. Fica
          indisponível durante uma reconexão.</span></li>
        <li><span><b>Nota privada:</b> pelo menu do assento, registre uma leitura sobre o adversário e marque com uma
          cor. Só você vê o texto e o ponto colorido.</span></li>
        <li><span><b>Últimos vencedores:</b> resumo das últimas mãos resolvidas nesta mesa.</span></li>
      </GuideBullets>
        <p>O mesmo menu do assento tem adicionar amigo, silenciar, bloquear, denunciar e abrir a vitrine do
          jogador.</p></>
    },
    {
      id: 'preferencias',
      title: 'Preferências da mesa',
      summary: 'Apresentação, som e ritmo. Nada aqui muda as regras da mão.',
      image: {src: '/guide/table-preferences.webp', alt: 'Diálogo de preferências com tema do feltro, sons, dealer, comandos por voz, treinador e lembrete'},
      body: <><GuideBullets>
        <li><span><b>Tema do feltro:</b> Clássico vem liberado; Meia-noite, Bordô e Oceano são cosméticos da Loja e
          levam até lá quando ainda não são seus. A escolha fica na sua conta e vale em qualquer mesa.</span></li>
        <li><span><b>Sons da mesa:</b> cartas, fichas e alertas. Começa desligado. Ao ligar, os áudios são
          carregados na hora, então o aviso de &ldquo;sua vez&rdquo; toca sem atraso mesmo em conexão lenta.</span></li>
        <li><span><b>Dealer auditivo:</b> narra as cartas e as ações principais.</span></li>
        <li><span><b>Rodar duas vezes:</b> aparece somente nas salas que permitem o recurso.</span></li>
        <li><span><b>Comandos por voz:</b> push-to-talk para Fold, Check, Pagar, Aumentar e All In. O jogo recebe a
          ação reconhecida, nunca o áudio. Fold, Check e Pagar valem na hora; Aumentar e All In abrem um chip de
          confirmação — diga &ldquo;confirmar&rdquo; ou toque em <b>Confirmar</b>. Sem confirmação em alguns segundos, o
          aumento é descartado sozinho.</span></li>
        <li><span><b>Treinador:</b> explica sua mão depois que você age, só em mesas sandbox. Fica indisponível
          durante a sua decisão, para nunca funcionar como dica em tempo real; ao fim da mão, mostra como a sua chance
          mudou rua a rua.</span></li>
        <li><span><b>Lembrete de sessão:</b> a cada 30, 60, 90 ou 120 minutos, ou desativado.</span></li>
      </GuideBullets>
        <p>O tema do feltro e o baralho ficam salvos na sua conta. As outras preferências valem neste
          navegador.</p></>
    },
    {
      id: 'pausar-sair',
      title: 'Pausar, recomprar e sair',
      summary: 'Sair não interrompe a mão em andamento, e dá para voltar atrás.',
      body: <><GuideBullets>
        <li><span><b>Sentar fora:</b> você continua no assento e volta quando quiser. O selo avisa a mesa que a pausa
          começa na próxima mão.</span></li>
        <li><span><b>Recompra:</b> com a pilha zerada e a participação pausada, o diálogo oferece um novo buy-in
          dentro dos limites da mesa, com auto rebuy e um atalho para resgatar as fichas grátis do dia.</span></li>
        <li><span><b>Sair da mesa:</b> fora de uma mão, a saída é imediata; dentro dela, fica marcada e acontece no
          fim. Até lá, <b>Cancelar saída</b> desfaz o pedido.</span></li>
        <li><span><b>Resumo da sessão:</b> ao sair, um resumo mostra tempo na mesa, entrada, mãos jogadas, maior pote
          ganho e o resultado.</span></li>
        <li><span><b>Inatividade:</b> no último minuto antes da remoção, um aviso conta o tempo e oferece
          <b> Continuar na mesa</b>.</span></li>
      </GuideBullets></>
    }
  ]}/>;
}
