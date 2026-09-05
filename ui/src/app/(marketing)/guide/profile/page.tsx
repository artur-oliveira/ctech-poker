'use client';

import {UserRound} from 'lucide-react';
import {GuideBullets, GuideCallout, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function ProfileGuide() {
  return <GuidePage icon={UserRound} eyebrow="IDENTIDADE E LEITURA" title="Perfil, vitrine e estatísticas"
    description="Como você aparece na mesa, o que fica público e o que as suas mãos dizem sobre o seu jogo."
    currentHref="/guide/profile" next={{href: '/guide/community', label: 'Comunidade e segurança'}} sections={[
      {
        id: 'menu', title: 'O menu do perfil', summary: 'O avatar no topo reúne identidade, baralho, carteiras e atalhos pessoais.',
        image: {src: '/guide/profile-live.webp', alt: 'Menu do perfil com foto, nome, baralho, saldo sandbox e atalhos'},
        body: <><GuideBullets><li><span>O saldo ao lado do avatar abre a Loja direto.</span></li>
          <li><span><b>Baralho</b> muda as cartas exibidas para você.</span></li>
          <li><span>O menu mostra o seu saldo sandbox. O modo dinheiro real, e o controle para alterná-lo, só aparecem em ambientes onde as apostas em dinheiro real estão liberadas — o padrão é sandbox.</span></li>
          <li><span><b>Seu jogo</b> e <b>Vitrine do perfil</b> abrem as duas telas pessoais descritas nesta página.</span></li>
          <li><span><b>Sair da conta</b> encerra a sessão de autenticação.</span></li></GuideBullets>
          <p>A linha sob o seu nome diz, o tempo todo, se a vitrine está pública ou privada.</p></>
      },
      {
        id: 'identidade', title: 'Nome e foto', summary: 'É assim que a mesa, a vitrine e o ranking chamam você.',
        body: <><GuideSteps><li><span>Toque no nome atual para editar. Enter confirma, Escape cancela.</span></li>
          <li><span>Na foto, use a câmera para enviar um JPG ou PNG; a lixeira remove a imagem.</span></li></GuideSteps>
          <p>Sem foto, o jogo usa suas iniciais. O envio mostra o estado de progresso e avisa se falhar. Trocar de nome não afeta mãos já registradas.</p></>
      },
      {
        id: 'baralho', title: 'Escolher o baralho', summary: 'Preferência visual, aplicada às cartas que você vê.',
        body: <><p>O seletor mostra uma amostra dos quatro ases de cada variante. As gratuitas são aplicadas na hora; as premium aparecem com cadeado, com o preço, e levam à seção de baralhos da Loja.</p>
          <GuideCallout kind="info" title="Somente apresentação">Trocar o baralho não interfere na distribuição, no hash da prova nem nas cartas que você recebe.</GuideCallout></>
      },
      {
        id: 'vitrine', title: 'Configurar a vitrine pública', summary: 'A vitrine começa privada e só mostra o que você habilitar.',
        body: <><GuideTerms><GuideTerm term="Perfil público">Libera a rota compartilhável da sua vitrine. Enquanto está desligada, o link não abre para ninguém.</GuideTerm>
          <GuideTerm term="Estilo de jogo público">Depois de 200 mãos, mostra um rótulo de tendência na mesa e na vitrine. Em vitrine pública, ele pode ser visto sem login.</GuideTerm>
          <GuideTerm term="Mesa visível para amigos">Permite que amigos entrem na sua mesa quando ela é pública. Mesa privada nunca aparece.</GuideTerm>
          <GuideTerm term="Conquistas em destaque">Até três conquistas já iniciadas.</GuideTerm>
          <GuideTerm term="Organizar vitrine">Reordene Conquistas, Melhor Vitória e Cara a Cara com as setas para cima e para baixo — sem
            arrastar, funciona igual por toque, mouse ou teclado. Melhor Vitória e Cara a Cara também podem ser escondidos; Conquistas
            sempre aparece.</GuideTerm>
          <GuideTerm term="Copiar link / Ver perfil">Aparecem com a vitrine pública e mostram exatamente o que a outra pessoa recebe.</GuideTerm></GuideTerms>
          <p>Quem abre a sua vitrine vê nome, foto, os rótulos de estilo autorizados e as seções que você deixou visíveis, na ordem que
            você escolheu: conquistas em destaque, a sua melhor vitória recente e — se estiver logado — o &quot;Cara a Cara&quot;: quantas
            mãos vocês jogaram juntos, quantas cada um venceu, quantas empataram e o saldo de fichas do confronto.</p>
          <GuideCallout kind="safe" title="Estatística detalhada não vai junto">Tornar a vitrine pública não publica VPIP, PFR nem 3-bet. Só os rótulos que você autorizou saem do seu perfil.</GuideCallout>
          <p>Abrir o seu próprio link de vitrine mostra o botão &quot;Editar minha vitrine&quot; em vez do perfil público. Uma vitrine privada de outro jogador diz que está privada — não que o perfil não existe.</p></>
      },
      {
        id: 'hud', title: 'Ler “Seu jogo”', summary: 'Tendências pré-flop calculadas a partir das suas mãos concluídas.',
        body: <><GuideTerms><GuideTerm term="VPIP">Mãos em que você colocou fichas voluntariamente no pote pré-flop, sem contar os blinds.</GuideTerm>
          <GuideTerm term="PFR">Mãos em que você fez pelo menos um raise pré-flop.</GuideTerm>
          <GuideTerm term="3-bet">Reaumentos diante de um raise, medidos sobre as oportunidades reais.</GuideTerm>
          <GuideTerm term="Radar e badges">Leitura da amostra em participação, iniciativa, pressão, reaumento e seleção. Abra um badge para ver o critério.</GuideTerm></GuideTerms>
          <p>As abas separam sandbox de dinheiro real, e o tamanho da amostra fica visível: com poucas mãos, leia os números como uma primeira impressão.</p>
          <GuideCallout kind="safe" title="Estatísticas privadas">A tela “Seu jogo” é visível só para você, em qualquer situação.</GuideCallout></>
      }
    ]}/>;
}
