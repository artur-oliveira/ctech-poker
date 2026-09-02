'use client';

import {Award} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function AchievementsGuide() {
  return <GuidePage icon={Award} eyebrow="PROGRESSO DO JOGADOR" title="Como funcionam as conquistas"
    description="Cada meta acumula progresso a partir das mãos concluídas e libera estrelas em marcos sucessivos."
    currentHref="/guide/achievements" next={{href: '/guide/store', label: 'Conhecer a Loja'}} sections={[
      {
        id: 'progresso', title: 'Estrelas, maestria e a próxima meta', summary: 'Uma conquista tem vários níveis, não apenas concluída ou não.',
        image: {src: '/guide/achievements-live.webp', alt: 'Catálogo de conquistas com maestria geral, próxima estrela, filtros e cartões em progresso'},
        body: <><GuideTerms><GuideTerm term="Contagem">Quantidade registrada para a meta: mãos jogadas, vitórias, all-ins e assim por diante.</GuideTerm>
          <GuideTerm term="Estrelas">Cada marco alcançado preenche uma estrela, até cinco. O requisito do próximo nível fica no cartão.</GuideTerm>
          <GuideTerm term="Maestria geral">Percentual de estrelas conquistadas entre todas as visíveis no modo selecionado.</GuideTerm>
          <GuideTerm term="Sua próxima estrela">A meta em que você está mais perto do próximo nível, com quanto falta.</GuideTerm></GuideTerms>
          <p>Passe o ponteiro, toque ou navegue pelo teclado em um cartão para ver a descrição e os requisitos de cada nível. O catálogo abre sem login; o progresso exige a sua conta.</p></>
      },
      {
        id: 'categorias', title: 'O que gera progresso', summary: 'O catálogo cobre frequência, resultado, combinações e situações raras.',
        body: <><GuideBullets><li><span><b>Volume:</b> mãos jogadas, sequência na mesma mesa e fichas ganhas.</span></li>
          <li><span><b>Resultados:</b> vitórias, empates, showdowns, blefes, viradas e derrotas específicas.</span></li>
          <li><span><b>Combinações:</b> vencer com par, full house, straight flush e as demais categorias.</span></li>
          <li><span><b>Contexto:</b> heads-up, mesa cheia, all-in contra pilha maior, pocket aces quebrados, decisões no river e mãos ganhas sem olhar as cartas.</span></li></GuideBullets>
          <GuideCallout kind="info" title="A mão precisa terminar">O progresso vem do registro confirmado da mão. Sair no meio ou abrir uma simulação não conta.</GuideCallout></>
      },
      {
        id: 'filtros', title: 'Filtrar e separar carteiras', summary: 'Sandbox e dinheiro real acumulam progresso independente.',
        body: <><p>Escolha a carteira nas abas e depois filtre por <b>Todas</b>, <b>Desbloqueadas</b>, <b>Em progresso</b> ou <b>Completas</b> — cada filtro traz a contagem junto. Metas ligadas a um único modo, como fichas sandbox ganhas, só aparecem nele.</p>
          <GuideCallout kind="warning" title="Carteira real pode estar indisponível">Ver a aba não significa que o dinheiro real esteja liberado. O ambiente padrão é sandbox e a ativação depende do serviço.</GuideCallout></>
      },
      {
        id: 'secretas', title: 'Conquistas secretas e avisos', summary: 'Metas secretas entram no catálogo depois do primeiro marco.',
        body: <><p>Algumas situações raras não são antecipadas. Ao alcançar o primeiro nível, a conquista passa a aparecer no catálogo e um aviso de desbloqueio surge onde você estiver — inclusive na mesa, sem cobrir o resultado da mão.</p>
          <p>O aviso conta o evento; a página de Conquistas continua sendo a fonte do progresso total e do próximo requisito.</p></>
      },
      {
        id: 'vitrine', title: 'Destacar conquistas no perfil', summary: 'Até três conquistas com progresso compõem sua vitrine pública.',
        body: <><p>No menu do perfil, abra <b>Vitrine do perfil</b>, escolha até três conquistas já iniciadas, defina se a vitrine é pública e salve. Só o que você marca aparece para os outros.</p>
          <GuideLink href="/achievements">Abrir minhas conquistas</GuideLink></>
      }
    ]}/>;
}
