'use client';

import {UserRound} from 'lucide-react';
import {GuideBullets, GuideCallout, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function ProfileGuide() {
  return <GuidePage icon={UserRound} eyebrow="IDENTIDADE E LEITURA" title="Perfil, vitrine e estatísticas"
    description="Personalize como você aparece na mesa, escolha o que fica público e acompanhe tendências privadas do seu jogo."
    currentHref="/guide/profile" next={{href: '/guide/community', label: 'Comunidade e segurança'}} sections={[
      {
        id: 'menu', title: 'Abrir e entender o menu', summary: 'O avatar no topo reúne identidade, baralho, carteiras e atalhos pessoais.',
        image: {src: '/guide/profile-live.webp', alt: 'Menu do perfil com foto, nome, baralho, saldos e atalhos'},
        body: <><GuideBullets><li><span>O saldo ao lado do avatar abre diretamente Fichas.</span></li>
          <li><span><b>Fichas sandbox</b> e <b>Dinheiro real</b> ficam separados; trocar o modo não converte valores.</span></li>
          <li><span><b>Créditos e recompensas</b>, <b>Vitrine do perfil</b> e <b>Seu jogo</b> levam a funções diferentes.</span></li>
          <li><span><b>Sair da conta</b> encerra a sessão de autenticação atual.</span></li></GuideBullets></>
      },
      {
        id: 'identidade', title: 'Alterar nome e foto', summary: 'Mudanças confirmadas passam a identificar você nas mesas e superfícies públicas permitidas.',
        body: <><GuideSteps><li><span>Toque no avatar e depois no nome atual para editar.</span></li>
          <li><span>Digite o nome de exibição e salve; Enter também confirma e Escape cancela.</span></li>
          <li><span>Em <b>Foto do perfil</b>, adicione JPG ou PNG com corte quadrado, troque o arquivo ou remova a imagem.</span></li></GuideSteps>
          <p>Sem foto, o sistema usa o componente de avatar com suas iniciais. Uploads mostram estado de envio e uma mensagem informa sucesso ou falha.</p></>
      },
      {
        id: 'baralho', title: 'Escolher o visual do baralho', summary: 'A preferência altera a leitura visual das cartas sem mudar a mão.',
        body: <><p>O seletor mostra uma amostra dos quatro ases para cada variante. A escolha fica associada ao perfil e é aplicada às cartas exibidas para você.</p>
          <GuideCallout kind="info" title="Somente apresentação">Trocar o baralho não interfere na distribuição, no hash da prova ou nas cartas recebidas.</GuideCallout></>
      },
      {
        id: 'vitrine', title: 'Configurar a vitrine pública', summary: 'A vitrine começa privada e só expõe os blocos que você habilitar.',
        body: <><GuideTerms><GuideTerm term="Perfil público">Permite abrir a rota compartilhável da sua vitrine.</GuideTerm>
          <GuideTerm term="Estilo de jogo público">Autoriza mostrar badges derivados das suas estatísticas.</GuideTerm>
          <GuideTerm term="Conquistas em destaque">Seleciona até três conquistas com progresso para aparecerem no perfil.</GuideTerm>
          <GuideTerm term="Copiar link / Ver perfil">Só aparecem quando a vitrine está pública; servem para compartilhar e revisar a mesma visualização que outra pessoa recebe.</GuideTerm></GuideTerms>
          <GuideCallout kind="safe" title="Controle de privacidade">Nome e foto usados no jogo podem aparecer em contextos sociais. Estatísticas detalhadas continuam privadas; apenas badges autorizados entram na vitrine.</GuideCallout></>
      },
      {
        id: 'hud', title: 'Ler “Seu jogo”', summary: 'O HUD pessoal calcula tendências pré-flop a partir das mãos concluídas.',
        body: <><GuideTerms><GuideTerm term="VPIP">Percentual de mãos em que você colocou fichas voluntariamente no pote pré-flop, sem contar blinds.</GuideTerm>
          <GuideTerm term="PFR">Percentual de mãos em que fez pelo menos um raise pré-flop.</GuideTerm>
          <GuideTerm term="3-bet">Frequência de reaumento diante de um raise, medida nas oportunidades reais.</GuideTerm>
          <GuideTerm term="Radar e badges">Interpretação da amostra em participação, iniciativa, pressão, reaumento e seleção. Abra um badge para ver o critério.</GuideTerm></GuideTerms>
          <p>As abas mantêm estatísticas de Sandbox e Dinheiro real separadas. Com poucas mãos, leia os números como amostra inicial; eles ficam mais representativos conforme você joga.</p>
          <GuideCallout kind="safe" title="Estatísticas privadas">A tela “Seu jogo” é visível somente para você. Tornar a vitrine pública não publica automaticamente VPIP, PFR ou 3-bet.</GuideCallout></>
      }
    ]}/>;
}
