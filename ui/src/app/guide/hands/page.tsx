'use client';

import {History} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function HandsGuide() {
  return <GuidePage icon={History} eyebrow="MEMÓRIA DA PARTIDA" title="Mãos, replay e Provably Fair"
    description="Encontre uma rodada, reconstrua cada decisão e confira no próprio navegador se as cartas pertencem ao baralho comprometido."
    currentHref="/guide/hands" next={{href: '/guide/achievements', label: 'Entender conquistas'}} sections={[
      {
        id: 'historico', title: 'Encontrar uma mão', summary: 'O histórico separa carteiras e resume resultado, board e combinação.',
        image: {src: '/guide/hands-live.webp', alt: 'Histórico de mãos com cartas, board, resultado e filtros'},
        body: <><p>Abra <b>Mãos</b> na navegação principal. Alterne entre Sandbox e Dinheiro real para consultar registros separados; uma carteira vazia não apaga os dados da outra.</p>
          <GuideBullets><li><span>Os indicadores somam as mãos já carregadas, saldo líquido e taxa de vitórias.</span></li>
            <li><span>Filtros exibem todas, vitórias/empates ou derrotas.</span></li>
            <li><span>Ao chegar ao fim, a próxima página carrega automaticamente; o botão manual permanece disponível para teclado e recuperação.</span></li>
            <li><span>Cada linha informa se a seed completa foi revelada ou se há uma prova parcial.</span></li></GuideBullets></>
      },
      {
        id: 'detalhes', title: 'Ler os detalhes', summary: 'A página preserva o que foi público na mão e organiza a sequência cronológica.',
        body: <><GuideTerms><GuideTerm term="Resultado líquido">Diferença entre o que saiu e voltou para sua pilha naquela mão.</GuideTerm>
          <GuideTerm term="Jogadores">Mostra suas cartas e apenas as cartas adversárias realmente reveladas.</GuideTerm>
          <GuideTerm term="Board comunitário">As cinco posições do flop ao river, inclusive quando a mão terminou antes de completar o board.</GuideTerm>
          <GuideTerm term="Histórico de ações">Entrada, blinds, check, fold, call, bet, raise, all-in, chat, reações, vencedores e empates em ordem de tempo.</GuideTerm></GuideTerms>
          <GuideCallout kind="safe" title="Informação oculta continua privada">Se um adversário não mostrou as cartas, o detalhe apresenta versos ou o aviso “Cartas não reveladas”. Nenhum cálculo tenta adivinhá-las.</GuideCallout></>
      },
      {
        id: 'replay', title: 'Assistir ao replay', summary: 'O replayer usa frames registrados na mão para avançar ação por ação.',
        body: <><GuideSteps><li><span>Abra uma mão e escolha <b>Assistir replay</b> quando o registro tiver frames.</span></li>
          <li><span>Use reproduzir/pausar, ação anterior, próxima ação ou voltar ao início.</span></li>
          <li><span>Altere a velocidade para revisar decisões longas ou acompanhar cada detalhe.</span></li>
          <li><span>Volte aos detalhes pelo link do próprio replayer; ele abre na mesma aba de propósito.</span></li></GuideSteps></>
      },
      {
        id: 'fair', title: 'Verificar a prova de integridade', summary: 'A checagem criptográfica roda localmente e não aceita um selo pronto do servidor.',
        body: <><p>Antes da distribuição, a mesa compromete uma representação do baralho por hash. Depois, a prova revela dados suficientes para o navegador recalcular esse compromisso.</p>
          <GuideTerms><GuideTerm term="Prova completa">A seed do servidor recria o baralho; o navegador calcula o SHA-256 e compara com o hash comprometido.</GuideTerm>
          <GuideTerm term="Prova parcial">Cada posição revelada inclui carta e salt; posições ocultas permanecem apenas como hashes. A raiz confirma que todas pertencem ao mesmo compromisso sem expor cartas privadas.</GuideTerm>
          <GuideTerm term="Correspondência">Indica que o cálculo local reproduziu o compromisso. Divergência ou dados ausentes aparecem explicitamente, nunca como “verificado”.</GuideTerm></GuideTerms>
          <GuideCallout kind="info" title="Integridade não prevê o futuro">A prova confirma consistência do baralho registrado. Ela não revela cartas ainda ocultas nem muda o resultado da mão.</GuideCallout></>
      },
      {
        id: 'compartilhar', title: 'Compartilhar ou baixar', summary: 'Escolha entre um link público limitado e um arquivo de análise pessoal.',
        body: <><GuideBullets><li><span><b>Compartilhar:</b> cria um link público com resultado, suas cartas, board e resumo. Revise o aviso antes de criar.</span></li>
          <li><span><b>Copiar:</b> envia o link gerado à área de transferência; quem recebe não precisa navegar pelo seu histórico privado.</span></li>
          <li><span><b>Exportar .txt:</b> baixa um relato legível com dados da mão e ações disponíveis para estudo ou arquivo.</span></li></GuideBullets>
          <GuideCallout kind="warning" title="O link é público">Compartilhe somente quando estiver confortável em expor as informações descritas no diálogo. Não inclua informação que permaneceu oculta no jogo.</GuideCallout>
          <GuideLink href="/hands">Abrir minhas mãos</GuideLink></>
      }
    ]}/>;
}
