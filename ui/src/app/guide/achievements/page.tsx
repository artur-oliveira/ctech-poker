'use client';

import {Award} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function AchievementsGuide() {
  return <GuidePage icon={Award} eyebrow="PROGRESSO DO JOGADOR" title="Como funcionam as conquistas"
    description="Metas acumulam progresso a partir das mãos concluídas e liberam estrelas em marcos sucessivos."
    currentHref="/guide/achievements" next={{href: '/guide/store', label: 'Ganhar fichas diárias'}} sections={[
      {
        id: 'progresso', title: 'Estrelas, níveis e maestria', summary: 'Uma conquista pode ter vários marcos, não apenas um estado concluído.',
        image: {src: '/guide/achievements-live.webp', alt: 'Catálogo de conquistas com estatísticas, filtros e progresso em estrelas'},
        body: <><GuideTerms><GuideTerm term="Contagem">Quantidade registrada para a meta, como mãos jogadas, vitórias ou all-ins.</GuideTerm>
          <GuideTerm term="Estrelas">Cada marco alcançado preenche uma estrela. O requisito do próximo nível aparece no cartão.</GuideTerm>
          <GuideTerm term="Completa">Todos os marcos disponíveis daquela conquista foram alcançados.</GuideTerm>
          <GuideTerm term="Maestria geral">Percentual de estrelas conquistadas entre todas as estrelas visíveis no modo selecionado.</GuideTerm></GuideTerms>
          <p>Passe o ponteiro, toque ou navegue pelo teclado no cartão para conferir a descrição e os requisitos de cada nível.</p></>
      },
      {
        id: 'categorias', title: 'O que pode gerar progresso', summary: 'O catálogo cobre frequência, resultado, combinações e situações raras.',
        body: <><GuideBullets><li><span><b>Volume:</b> mãos jogadas, sequência na mesma mesa e fichas ganhas.</span></li>
          <li><span><b>Resultados:</b> vitórias, empates, showdowns, blefes, viradas e derrotas específicas.</span></li>
          <li><span><b>Combinações:</b> vencer com par, full house, straight flush e demais categorias.</span></li>
          <li><span><b>Contexto:</b> heads-up, mesa cheia, all-in contra stack maior, pocket aces quebrados e jogadas no river.</span></li></GuideBullets>
          <GuideCallout kind="info" title="A mão precisa terminar">O progresso nasce do registro confirmado da mão. Sair no meio ou olhar uma simulação não cria contagem.</GuideCallout></>
      },
      {
        id: 'filtros', title: 'Filtrar e separar carteiras', summary: 'Sandbox e dinheiro real mantêm progresso independente quando o serviço suporta ambos.',
        body: <><p>Use as abas de carteira e depois filtre por <b>Todas</b>, <b>Desbloqueadas</b>, <b>Em progresso</b> ou <b>Completas</b>. Algumas metas, como fichas sandbox ganhas, pertencem a um único modo.</p>
          <GuideCallout kind="warning" title="Carteira real pode estar indisponível">A interface não significa que dinheiro real esteja liberado. O ambiente padrão é sandbox e a ativação depende do serviço.</GuideCallout></>
      },
      {
        id: 'secretas', title: 'Conquistas secretas e notificações', summary: 'Metas secretas só entram no catálogo depois do primeiro marco.',
        body: <><p>O sistema evita antecipar certas situações raras. Quando você alcança o primeiro nível, a conquista passa a aparecer e uma notificação de desbloqueio pode surgir durante a experiência.</p>
          <p>As notificações informam o evento; a página de Conquistas continua sendo a fonte para progresso total e próximos requisitos.</p></>
      },
      {
        id: 'vitrine', title: 'Destacar conquistas no perfil', summary: 'Até três conquistas com progresso podem compor sua vitrine pública.',
        body: <><p>No menu do perfil, abra <b>Vitrine do perfil</b>. Selecione conquistas já iniciadas, defina se a vitrine é pública e salve. A contagem pública respeita as escolhas da vitrine.</p>
          <GuideLink href="/achievements">Abrir minhas conquistas</GuideLink></>
      }
    ]}/>;
}
