'use client';

import {History} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function HandsGuide() {
  return <GuidePage icon={History} eyebrow="MEMÓRIA DA PARTIDA" title="Mãos, replay e Provably Fair"
    description="Encontre uma rodada, reconstrua cada decisão e confira no próprio navegador se as cartas vieram do baralho comprometido."
    currentHref="/guide/hands" next={{href: '/guide/achievements', label: 'Entender conquistas'}} sections={[
      {
        id: 'historico', title: 'Encontrar uma mão', summary: 'A lista separa as carteiras e resume cartas, board e resultado.',
        image: {src: '/guide/hands-live.webp', alt: 'Lista de mãos com cartas, board, combinação, resultado e estado da seed'},
        body: <><p>Abra <b>Mãos</b> na navegação. As abas Sandbox e Dinheiro real consultam registros independentes; uma carteira vazia não apaga a outra.</p>
          <GuideBullets><li><span>Os três indicadores no topo resumem só o que está <b>nesta lista</b>: quantidade, saldo e taxa de vitórias com vitórias, empates e derrotas — eles acompanham o filtro ativo.</span></li>
            <li><span>Logo abaixo, a faixa <b>Desde o início</b> traz os números de toda a sua história na carteira escolhida, para você não confundir o recorte carregado com o total. Ela aparece quando você já tem posição no ranking.</span></li>
            <li><span>Os filtros <b>Todas</b>, <b>Só vitórias</b>, <b>Só derrotas</b> e <b>Só empates</b> reorganizam as mãos já carregadas sem buscar nada de novo; havendo mais de uma mesa, uma segunda linha de filtros deixa escolher <b>Todas as mesas</b> ou uma delas.</span></li>
            <li><span>As mãos vêm agrupadas por dia — <b>Hoje</b>, <b>Ontem</b> e a data completa para as mais antigas — e o dia em foco fica fixo no alto enquanto você rola.</span></li>
            <li><span>Sem filtro, a lista carrega sozinha conforme você desce. Com um filtro por resultado ou por mesa ativo, o carregamento passa a ser só pelo botão <b>Carregar mais mãos</b> — assim o filtro não puxa o histórico inteiro de uma vez.</span></li>
            <li><span>Cada linha diz se a seed do servidor já foi revelada ou se a mão ainda tem prova parcial, e mostra o nível de blinds daquela mão quando o registro o guarda.</span></li>
            <li><span>Se o filtro não deixar nenhuma mão, a página explica o que aconteceu e oferece <b>Limpar filtros</b>. Sem nenhuma mão registrada, ela mostra o caminho de volta ao lobby em vez de uma lista vazia.</span></li>
            <li><span>Acima dos filtros, escolha entre <b>Filtros</b> e <b>Coleções</b>. Em Filtros, depois de ajustar resultado e mesa, dê um nome em <b>Salvar filtro atual como</b> para guardar essa combinação — ela reaparece como um atalho ao lado dos demais, em qualquer aparelho, e pode ser removida com o <b>×</b>.</span></li>
            <li><span>Em Coleções, cada mão que você marcou — pelo próprio detalhe da mão, ou pela marca de revisão — aparece como um atalho nomeado. A coleção <b>Marcadas para revisar</b> reúne automaticamente tudo que você marcou como "para revisar"; abrir uma coleção filtra a lista só pelas mãos dela.</span></li></GuideBullets></>
      },
      {
        id: 'detalhes', title: 'Ler os detalhes', summary: 'A página preserva o que foi público na mão e organiza a sequência.',
        body: <><GuideTerms><GuideTerm term="Resultado líquido">Diferença entre o que saiu e o que voltou para a sua pilha naquela mão.</GuideTerm>
          <GuideTerm term="Jogadores">Suas cartas e apenas as cartas adversárias realmente reveladas.</GuideTerm>
          <GuideTerm term="Cartas comunitárias">As cinco posições do flop ao river, inclusive quando a mão terminou antes de completar o board.</GuideTerm>
          <GuideTerm term="Histórico de ações">Entrada, blinds, check, fold, call, bet, raise, all-in, chat, reações, vencedores e empates em ordem de tempo.</GuideTerm>
          <GuideTerm term="Mesa">O identificador da sala, com um botão para copiar — útil para comparar duas mãos da mesma mesa.</GuideTerm></GuideTerms>
          <GuideCallout kind="safe" title="Informação oculta continua privada">Quando um adversário não mostrou as cartas, o detalhe apresenta versos e o aviso “Cartas não reveladas”. Nada é deduzido.</GuideCallout>
          <p>O nome e o avatar de cada adversário abrem o perfil público dele, e o botão <b>⋮</b> ao lado traz o mesmo menu de jogador das mesas e do
            <b> /people</b>: adicionar amigo, silenciar, bloquear, denunciar (já levando a mão em questão) e escrever uma nota privada.
            Sua própria carteira nunca mostra esse menu, e ele só aparece com uma conta autenticada.</p>
          <p>Se a sequência de ações falhar em carregar, o resumo, a prova e as ferramentas continuam na tela com uma nova tentativa só para ela.</p>
          <p>No topo, ao lado de <b>Exportar</b> e <b>Compartilhar</b>, o botão <b>Marcar para revisar</b> sinaliza a mão para
            estudar depois — ela aparece assim na coleção <b>Marcadas para revisar</b> em <b>Mãos</b>. Dentro do histórico de
            ações, cada etapa (pré-flop, flop, turn, river, showdown) tem um campo <b>Adicionar nota</b> para uma anotação
            curta sobre aquela decisão; o campo some sozinho enquanto vazio e guarda o que você escreveu assim que sai dele.
            Se salvar falhar, o texto continua no campo e um aviso explica o que houve.</p></>
      },
      {
        id: 'replay', title: 'Assistir ao replay', summary: 'O replayer reconstrói a mesa a partir dos quadros gravados na mão.',
        image: {src: '/guide/hand-replay.webp', alt: 'Replay de uma mão com mesa reconstruída, controles de reprodução e ação atual'},
        body: <><GuideSteps><li><span>No detalhe da mão, escolha <b>Assistir replay</b>. Ele aparece quando a mão tem quadros gravados.</span></li>
          <li><span>Use reproduzir, pausar, ação anterior, próxima ação ou voltar ao início.</span></li>
          <li><span>Alterne a velocidade entre 1×, 2× e 0,5× para correr até o desfecho ou acompanhar cada detalhe.</span></li>
          <li><span>Pelo teclado, com o replayer em foco: <b>barra de espaço</b> reproduz ou pausa,
            <b>seta esquerda</b> e <b>seta direita</b> andam uma ação, e <b>Home</b> volta ao início.</span></li>
          <li><span>As reações enviadas naquele momento reaparecem junto com a ação que as provocou.</span></li>
          <li><span>O replay termina no desfecho: depois que o board é completado, ele ainda mostra quem <b>venceu</b> e quem <b>perdeu a mão</b> antes de parar.</span></li></GuideSteps>
          <p>O cabeçalho mostra o <b>big blind</b> daquela mão ao lado do pote, para o tamanho das fichas fazer sentido
            em qualquer stake. Mãos antigas sem o valor registrado usam o big blind do lance inicial ou 25 como
            referência.</p>
          <p>O replayer abre na mesma aba de propósito: o link de volta leva direto ao detalhe da mão.</p>
          <p>Se o seu sistema estiver com <b>animações reduzidas</b>, o replay não corre: cada ação fica mais tempo na
            tela, no lugar da animação que marcava o ritmo.</p>
          <p>O botão <b>Modo Coaching</b>, no cabeçalho, é opcional e começa desligado. Com ele ativado, o replay
            pausa sozinho em cada decisão sua, faz uma pergunta rápida (pot odds, posição, range do vilão…) e só
            revela a ação real quando você pede — em <b>Já pensei, revelar ação</b> ou <b>Pular pergunta</b>.</p></>
      },
      {
        id: 'fair', title: 'Verificar a prova de integridade', summary: 'A checagem roda no seu navegador; nenhum selo pronto vem do servidor.',
        body: <><p>Antes de distribuir, a mesa compromete o baralho por hash. Depois, a prova revela o suficiente para o navegador recalcular esse compromisso e comparar.</p>
          <GuideTerms><GuideTerm term="Prova completa">A seed do servidor recria o baralho; o navegador calcula o SHA-256 e compara com o hash comprometido.</GuideTerm>
          <GuideTerm term="Prova parcial">Cada posição revelada traz carta e salt; as ocultas permanecem como hashes. A raiz confirma que todas pertencem ao mesmo compromisso sem expor carta nenhuma.</GuideTerm>
          <GuideTerm term="Correspondência">Diz que o cálculo local reproduziu o compromisso. Divergência ou dado faltando aparece como tal, nunca como “verificado”.</GuideTerm></GuideTerms>
          <GuideCallout kind="info" title="Integridade não prevê o futuro">A prova confirma que o baralho registrado é consistente. Ela não revela cartas ocultas nem muda o resultado.</GuideCallout>
          <p>Mãos anteriores à prova criptográfica dizem, em tom neutro, que <b>foram registradas antes da prova criptográfica</b> — não há nada a recalcular ali, e isso não é uma falha de verificação. O vermelho de falha continua reservado para quando o hash realmente não confere.</p></>
      },
      {
        id: 'compartilhar', title: 'Compartilhar ou exportar', summary: 'Um link público com prazo, ou um arquivo de texto para estudo.',
        body: <><GuideSteps><li><span>Em <b>Compartilhar</b>, escolha a história: <b>Brag</b> para a vitória, <b>Bad beat</b> para a derrota improvável.</span></li>
          <li><span>Decida se suas cartas entram. Desmarcado, o link publica só board, resultado e ações.</span></li>
          <li><span>Escolha por quanto tempo o link vive: 24 horas, 7 dias ou 30 dias.</span></li>
          <li><span>Crie e copie. Quem abrir vê uma versão anonimizada, sem acesso ao seu histórico.</span></li></GuideSteps>
          <p><b>Exportar .txt</b> baixa um relato legível da mão, com as ações disponíveis, para arquivo ou análise fora do jogo.</p>
          <GuideCallout kind="warning" title="O link é público enquanto durar">Qualquer pessoa com o endereço abre a mão até ela expirar. O que ficou oculto no jogo nunca entra no link.</GuideCallout>
          <p>Reabrir <b>Compartilhar</b> na mesma mão mostra o link já criado em vez de gerar outro — e traz o botão <b>Revogar</b>, que
            desativa esse link imediatamente e devolve a tela ao estado inicial. A lembrança do link fica só neste navegador: em outro
            aparelho, ou depois de limpar os dados do site, a tela volta a oferecer a criação de um novo.</p>
          <p>No fim da página <b>Mãos</b>, o painel <b>Meus links compartilhados</b> lista todos os links ativos que você criou — em
            qualquer aparelho — com o tipo, o resultado, quando foi criado e quando expira. Cada linha traz <b>Copiar link</b> e
            <b> Revogar</b>: revogar desativa o endereço na hora, e quem tentar abri-lo vê a mensagem de link revogado ou expirado.
            Sem links ativos, o painel explica como criar o primeiro.</p>
          <GuideLink href="/hands">Abrir minhas mãos</GuideLink></>
      }
    ]}/>;
}
