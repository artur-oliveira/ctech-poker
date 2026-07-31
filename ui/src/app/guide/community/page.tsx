'use client';

import {ShieldCheck} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function CommunityGuide() {
  return <GuidePage icon={ShieldCheck} eyebrow="COMUNIDADE E CONTROLE" title="Ranking, convivência e jogo seguro"
    description="Recursos que conectam jogadores, preservam contexto entre sessões e ajudam você a manter controle sobre tempo, privacidade e conexão."
    currentHref="/guide/community" sections={[
      {
        id: 'ranking', title: 'Ranking da comunidade', summary: 'A classificação usa desempenho registrado nas mãos e separa cada carteira.',
        image: {src: '/guide/leaderboard-live.webp', alt: 'Ranking da comunidade com posição do jogador, pódio e estatísticas'},
        body: <><GuideTerms><GuideTerm term="Posição">Ordem atual dentro da lista retornada para o modo selecionado.</GuideTerm>
          <GuideTerm term="Vitórias">Número de mãos vencidas no conjunto contabilizado.</GuideTerm>
          <GuideTerm term="Mãos jogadas">Tamanho da amostra usada para contextualizar o resultado.</GuideTerm>
          <GuideTerm term="Taxa de vitória">Vitórias divididas pelas mãos jogadas; não mede lucro nem força dos adversários.</GuideTerm></GuideTerms>
          <p>O topo recebe um pódio quando há pelo menos três jogadores. Sua própria linha ou posição é destacada quando estiver presente.</p>
          <GuideLink href="/leaderboard">Abrir o ranking</GuideLink></>
      },
      {
        id: 'convites', title: 'Convidar amigos e compartilhar', summary: 'Links têm escopos diferentes conforme o recurso.',
        body: <><GuideBullets><li><span><b>Link de mesa:</b> abre uma sala específica e pode incluir o código de uma mesa privada.</span></li>
          <li><span><b>Link de mão:</b> abre um resumo público criado por você, sem conceder acesso ao histórico inteiro.</span></li>
          <li><span><b>Link de perfil:</b> abre sua vitrine somente depois que ela foi tornada pública.</span></li></GuideBullets>
          <GuideCallout kind="warning" title="Revogue pelo estado, não pelo segredo">Tornar a vitrine privada bloqueia a vitrine; links de mesa podem deixar de funcionar quando a sala encerra. Ainda assim, evite publicar convites privados.</GuideCallout></>
      },
      {
        id: 'convivencia', title: 'Chat, reações e notas privadas', summary: 'O sistema diferencia expressão pública de anotações pessoais.',
        body: <><GuideTerms><GuideTerm term="Chat">Mensagens visíveis aos participantes da mesa e registradas no histórico de ações disponível.</GuideTerm>
          <GuideTerm term="Reações">Emotes rápidos ou objetos direcionados a um assento. Você pode silenciar as animações recebidas.</GuideTerm>
          <GuideTerm term="Notas de jogador">Texto privado associado ao adversário para sua própria referência. O outro jogador não recebe nem visualiza a nota.</GuideTerm></GuideTerms></>
      },
      {
        id: 'sessao', title: 'Pausa consciente e controle da sessão', summary: 'Um lembrete neutro resume tempo e resultado sem interromper sua decisão.',
        body: <><GuideSteps><li><span>Na mesa, abra Preferências e escolha 30, 60, 90 ou 120 minutos; também é possível desativar.</span></li>
          <li><span>Quando o intervalo termina, o aviso espera até você não estar na vez.</span></li>
          <li><span>Revise duração, buy-in, pilha atual, variação e mãos concluídas.</span></li>
          <li><span>Escolha continuar jogando ou use os controles normais para sentar fora/sair.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="Sandbox por padrão">Fichas sandbox são fictícias, mas tempo e comportamento continuam reais. Use o lembrete como ferramenta de ritmo, não como julgamento.</GuideCallout></>
      },
      {
        id: 'conexao', title: 'Reconexão, erros e proteção de ações', summary: 'A interface deixa de aceitar decisões até receber um estado confiável da mesa.',
        body: <><GuideBullets><li><span><b>Reconectando:</b> ações, chat e pré-seleções ficam protegidos enquanto o cliente recupera o snapshot.</span></li>
          <li><span><b>Tentar agora:</b> antecipa uma nova tentativa quando o aviso oferecer a ação.</span></li>
          <li><span><b>Ação rejeitada:</b> a barra explica o erro e volta a liberar decisões ainda válidas.</span></li>
          <li><span><b>Sessão expirada:</b> a autenticação precisa ser renovada; nenhuma ação é enviada sem token válido.</span></li></GuideBullets>
          <p>Se o navegador ficou offline perto do seu prazo, confirme o estado restaurado antes de interpretar o resultado. O servidor continua sendo a fonte da mão.</p></>
      },
      {
        id: 'protecao', title: 'Inatividade e verificação rápida', summary: 'Proteções automáticas aparecem somente quando existe um motivo concreto.',
        body: <><GuideTerms><GuideTerm term="Aviso de inatividade">No último minuto antes da remoção, mostra uma contagem e o botão <b>Continuar na mesa</b>. Sem confirmação, o assento pode ser liberado.</GuideTerm>
          <GuideTerm term="Verificação rápida">Uma sequência incomum de ações muito rápidas pode abrir uma checagem anti-bot. O relógio continua visível e o time bank permanece disponível.</GuideTerm>
          <GuideTerm term="Sessão expirada">Exige uma nova autenticação pela CTech Account; o cliente não tenta contornar ou armazenar uma sessão inválida.</GuideTerm></GuideTerms>
          <GuideCallout kind="info" title="A verificação não lê sua voz ou suas cartas">O desafio confirma interação humana. Comandos por voz enviam apenas a ação reconhecida, e cartas ocultas continuam fora da interface.</GuideCallout></>
      },
      {
        id: 'limites', title: 'O que o sistema não promete', summary: 'Conhecer os limites evita interpretar uma interface como garantia inexistente.',
        body: <><GuideBullets><li><span>O modo dinheiro real pode aparecer como opção, mas depende de uma liberação externa e está desligado por padrão.</span></li>
          <li><span>Não existem torneios, modo espectador, grade multi-mesa ou filtros avançados de lobby implementados.</span></li>
          <li><span>Provably Fair valida consistência criptográfica; não garante resultado favorável nem revela informação escondida.</span></li>
          <li><span>Ranking e badges descrevem a amostra disponível; não são aconselhamento financeiro nem avaliação definitiva de habilidade.</span></li></GuideBullets>
          <GuideLink href="/guide/basics">Voltar aos primeiros passos</GuideLink></>
      }
    ]}/>;
}
