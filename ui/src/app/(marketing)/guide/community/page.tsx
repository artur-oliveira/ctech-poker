'use client';

import {ShieldCheck} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function CommunityGuide() {
  return <GuidePage icon={ShieldCheck} eyebrow="COMUNIDADE E CONTROLE" title="Ranking, convivência e jogo seguro"
    description="Como encontrar gente para jogar, o que os outros veem de você e os controles de tempo, privacidade e conexão."
    currentHref="/guide/community" sections={[
      {
        id: 'ranking', title: 'Ranking da comunidade', summary: 'Classificação por desempenho registrado, com carteiras separadas.',
        image: {src: '/guide/leaderboard-live.webp', alt: 'Ranking com pódio, posição do jogador e estatísticas de vitórias'},
        body: <><GuideTerms><GuideTerm term="Posição">Ordem atual na lista do modo selecionado.</GuideTerm>
          <GuideTerm term="Vitórias">Mãos vencidas no conjunto contabilizado.</GuideTerm>
          <GuideTerm term="Mãos jogadas">Tamanho da amostra que contextualiza o resultado.</GuideTerm>
          <GuideTerm term="Taxa de vitória">Vitórias divididas por mãos jogadas. Não mede lucro nem a força dos adversários.</GuideTerm></GuideTerms>
          <p>Com três jogadores ou mais, o topo vira pódio. A sua linha fica destacada quando você aparece na lista,
            e a lista rola por toda a classificação sem travar, por mais longa que ela fique.</p>
          <p>O cartão &quot;Sua posição no ranking&quot; sempre mostra a sua colocação real entre todos os jogadores
            daquele modo — não apenas entre os que aparecem na página carregada. Se você ainda não jogou nenhuma mão
            nesse modo, ele mostra &quot;Ainda sem ranking&quot; em vez de uma posição.</p>
          <GuideLink href="/leaderboard">Abrir o ranking</GuideLink></>
      },
      {
        id: 'pessoas', title: 'Pessoas: amigos, solicitações e atividades', summary: 'A amizade é sempre mútua e só o código exato encontra alguém.',
        image: {src: '/guide/people-live.webp', alt: 'Página Pessoas com código de amizade, abas e lista de amigos'},
        body: <><GuideSteps><li><span>Abra <b>Pessoas</b> na navegação, ou o painel rápido do lobby para uma visão curta.</span></li>
          <li><span>Compartilhe seu código <code>PKR-XXXX-XXXX-XXXX</code> e busque o de quem você quer adicionar. Nome de exibição não é único e não encontra ninguém.</span></li>
          <li><span>A solicitação precisa ser aceita dos dois lados.</span></li></GuideSteps>
          <GuideBullets><li><span><b>Amigos:</b> presença e, quando a pessoa permitiu, um atalho para entrar na mesa dela.</span></li>
            <li><span><b>Solicitações:</b> recebidas e enviadas, com aceitar, recusar ou cancelar.</span></li>
            <li><span><b>Recentes:</b> quem sentou com você nos últimos 90 dias.</span></li>
            <li><span><b>Bloqueados:</b> a lista com o botão de desbloquear.</span></li>
            <li><span><b>Atividades:</b> solicitações e convites de mesa, com <b>Entrar</b> e <b>Recusar</b> na própria linha. É a única aba que zera o contador do menu.</span></li></GuideBullets>
          <GuideCallout kind="safe" title="Presença sem exposição">Amigos veem apenas online, offline ou em uma mesa. Blinds, saldo e o código de uma sala privada nunca aparecem — e a sua mesa só fica acessível se você ligar isso na vitrine.</GuideCallout>
          <GuideLink href="/people">Abrir Pessoas</GuideLink></>
      },
      {
        id: 'convites', title: 'Convidar e compartilhar', summary: 'Cada link tem um alcance diferente.',
        body: <><GuideBullets><li><span><b>Convite de mesa:</b> na mesa, <b>Convidar</b> copia o link e também chama amigos direto da sua lista. O convite vale 15 minutos, aparece nas atividades de quem recebeu e não reserva assento nem compra fichas.</span></li>
          <li><span><b>Link de mão:</b> abre um resumo público criado por você, com prazo, sem dar acesso ao seu histórico.</span></li>
          <li><span><b>Link de perfil:</b> abre a sua vitrine, e só enquanto ela estiver pública.</span></li></GuideBullets>
          <GuideCallout kind="warning" title="Revogue pelo estado, não pelo segredo">Tornar a vitrine privada fecha o perfil; um link de mão expira sozinho; um link de mesa deixa de valer quando a sala encerra. Ainda assim, evite publicar convites privados.</GuideCallout></>
      },
      {
        id: 'seguranca', title: 'Silenciar, bloquear e denunciar', summary: 'Tudo aqui é do observador e nada altera o jogo.',
        body: <><GuideTerms><GuideTerm term="Silenciar">Esconde chat e reações daquele jogador para você, em qualquer dispositivo. Ele não é avisado.</GuideTerm>
          <GuideTerm term="Bloquear">Inclui silenciar, desfaz a amizade e impede novas solicitações e convites. Desbloquear não reativa o conteúdo: ele segue silenciado até você mudar.</GuideTerm>
          <GuideTerm term="Denunciar">Escolha o motivo — assédio, discurso de ódio, spam, trapaça, nome ou avatar impróprio, ou outro — e descreva se quiser. A fila é revisada por pessoas e o denunciado não é avisado.</GuideTerm></GuideTerms>
          <p>As três ações estão no menu do assento na mesa e também na vitrine do jogador.</p>
          <GuideCallout kind="warning" title="Bloqueio não escolhe adversário">Um jogador bloqueado ainda pode cair na mesma mesa pública, e as apostas e ações dele continuam visíveis. Só o conteúdo social é suprimido.</GuideCallout></>
      },
      {
        id: 'sessao', title: 'Pausa consciente e controle da sessão', summary: 'Um lembrete neutro que nunca interrompe uma decisão.',
        body: <><GuideSteps><li><span>Na mesa, abra Preferências e escolha 30, 60, 90 ou 120 minutos — ou desative.</span></li>
          <li><span>No fim do intervalo, o aviso espera você não estar na vez.</span></li>
          <li><span>Ele resume tempo na mesa, mãos concluídas, entrada acumulada, pilha atual e resultado.</span></li>
          <li><span>Continue jogando ou use os controles normais para sentar fora ou sair.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="Fichas são fictícias; o tempo não">Use o lembrete como ferramenta de ritmo. Ao sair da mesa, o resumo da sessão fecha a conta da sua passagem por ela.</GuideCallout></>
      },
      {
        id: 'conexao', title: 'Reconexão, erros e proteção de ações', summary: 'A mesa não aceita decisões enquanto não tem estado confiável.',
        body: <><GuideBullets><li><span><b>Reconectando:</b> ações, chat e preparações ficam bloqueados até o snapshot chegar. Suas fichas continuam na mesa.</span></li>
          <li><span><b>Tentar agora:</b> antecipa uma nova tentativa; depois de várias falhas, o aviso passa a esperar esse toque em vez de prometer uma reconexão automática.</span></li>
          <li><span><b>Ação rejeitada:</b> a barra explica o erro e devolve as decisões ainda válidas.</span></li>
          <li><span><b>Removido da mesa:</b> por inatividade ou por tempo demais desconectado, com aviso e volta ao lobby; a pilha vai para a carteira.</span></li>
          <li><span><b>Sessão expirada:</b> a autenticação precisa ser renovada. Nenhuma ação é enviada sem token válido.</span></li></GuideBullets></>
      },
      {
        id: 'protecao', title: 'Inatividade e verificação rápida', summary: 'Proteções automáticas só aparecem com um motivo concreto.',
        body: <><GuideTerms><GuideTerm term="Aviso de inatividade">No último minuto antes da remoção, mostra a contagem e o botão <b>Continuar na mesa</b>.</GuideTerm>
          <GuideTerm term="Verificação rápida">Uma sequência incomum de ações muito rápidas pode abrir uma checagem anti-bot. O relógio da mesa continua visível e o time bank segue disponível.</GuideTerm></GuideTerms>
          <GuideCallout kind="info" title="A verificação não lê sua voz nem suas cartas">O desafio só confirma interação humana. Comandos por voz enviam apenas a ação reconhecida, e cartas ocultas continuam fora da interface.</GuideCallout></>
      },
      {
        id: 'limites', title: 'O que o sistema não promete', summary: 'Saber os limites evita ler garantias que não existem.',
        body: <><GuideBullets><li><span>O modo dinheiro real pode aparecer como opção, mas depende de liberação externa e está desligado por padrão.</span></li>
          <li><span>Não existem torneios, modo espectador, grade multi-mesa nem mensagens diretas.</span></li>
          <li><span>Provably Fair valida consistência criptográfica; não garante resultado favorável nem revela informação escondida.</span></li>
          <li><span>Ranking, badges e estilo descrevem a amostra disponível. Não são avaliação definitiva de habilidade.</span></li>
          <li><span>Denúncia nenhuma pune automaticamente por volume; toda decisão passa por revisão humana.</span></li></GuideBullets>
          <GuideLink href="/guide/basics">Voltar aos primeiros passos</GuideLink></>
      }
    ]}/>;
}
