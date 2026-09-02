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
          <GuideBullets><li><span>Os três indicadores no topo resumem as mãos já carregadas: quantidade, saldo e taxa de vitórias com vitórias, empates e derrotas.</span></li>
            <li><span>A lista carrega sozinha conforme você desce; o botão <b>Carregar mais mãos</b> continua disponível para teclado.</span></li>
            <li><span>Cada linha diz se a seed do servidor já foi revelada ou se a mão ainda tem prova parcial.</span></li>
            <li><span>Sem nenhuma mão registrada, a página mostra o caminho de volta ao lobby em vez de uma lista vazia.</span></li></GuideBullets></>
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
          <p>Se a sequência de ações falhar em carregar, o resumo, a prova e as ferramentas continuam na tela com uma nova tentativa só para ela.</p></>
      },
      {
        id: 'replay', title: 'Assistir ao replay', summary: 'O replayer reconstrói a mesa a partir dos quadros gravados na mão.',
        image: {src: '/guide/hand-replay.webp', alt: 'Replay de uma mão com mesa reconstruída, controles de reprodução e ação atual'},
        body: <><GuideSteps><li><span>No detalhe da mão, escolha <b>Assistir replay</b>. Ele aparece quando a mão tem quadros gravados.</span></li>
          <li><span>Use reproduzir, pausar, ação anterior, próxima ação ou voltar ao início.</span></li>
          <li><span>Alterne a velocidade entre 1× e 2× para correr até o desfecho ou acompanhar cada detalhe.</span></li>
          <li><span>As reações enviadas naquele momento reaparecem junto com a ação que as provocou.</span></li></GuideSteps>
          <p>O cabeçalho mostra o <b>big blind</b> daquela mão ao lado do pote, para o tamanho das fichas fazer sentido
            em qualquer stake. Mãos antigas sem o valor registrado usam o big blind do lance inicial ou 25 como
            referência.</p>
          <p>O replayer abre na mesma aba de propósito: o link de volta leva direto ao detalhe da mão.</p></>
      },
      {
        id: 'fair', title: 'Verificar a prova de integridade', summary: 'A checagem roda no seu navegador; nenhum selo pronto vem do servidor.',
        body: <><p>Antes de distribuir, a mesa compromete o baralho por hash. Depois, a prova revela o suficiente para o navegador recalcular esse compromisso e comparar.</p>
          <GuideTerms><GuideTerm term="Prova completa">A seed do servidor recria o baralho; o navegador calcula o SHA-256 e compara com o hash comprometido.</GuideTerm>
          <GuideTerm term="Prova parcial">Cada posição revelada traz carta e salt; as ocultas permanecem como hashes. A raiz confirma que todas pertencem ao mesmo compromisso sem expor carta nenhuma.</GuideTerm>
          <GuideTerm term="Correspondência">Diz que o cálculo local reproduziu o compromisso. Divergência ou dado faltando aparece como tal, nunca como “verificado”.</GuideTerm></GuideTerms>
          <GuideCallout kind="info" title="Integridade não prevê o futuro">A prova confirma que o baralho registrado é consistente. Ela não revela cartas ocultas nem muda o resultado.</GuideCallout></>
      },
      {
        id: 'compartilhar', title: 'Compartilhar ou exportar', summary: 'Um link público com prazo, ou um arquivo de texto para estudo.',
        body: <><GuideSteps><li><span>Em <b>Compartilhar</b>, escolha a história: <b>Brag</b> para a vitória, <b>Bad beat</b> para a derrota improvável.</span></li>
          <li><span>Decida se suas cartas entram. Desmarcado, o link publica só board, resultado e ações.</span></li>
          <li><span>Escolha por quanto tempo o link vive: 24 horas, 7 dias ou 30 dias.</span></li>
          <li><span>Crie e copie. Quem abrir vê uma versão anonimizada, sem acesso ao seu histórico.</span></li></GuideSteps>
          <p><b>Exportar .txt</b> baixa um relato legível da mão, com as ações disponíveis, para arquivo ou análise fora do jogo.</p>
          <GuideCallout kind="warning" title="O link é público enquanto durar">Qualquer pessoa com o endereço abre a mão até ela expirar. O que ficou oculto no jogo nunca entra no link.</GuideCallout>
          <GuideLink href="/hands">Abrir minhas mãos</GuideLink></>
      }
    ]}/>;
}
