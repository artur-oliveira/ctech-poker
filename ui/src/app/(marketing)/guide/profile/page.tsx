'use client';

import {UserRound} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function ProfileGuide() {
  return <GuidePage icon={UserRound} eyebrow="IDENTIDADE E LEITURA" title="Perfil, vitrine e estatísticas"
    description="Como você aparece na mesa, o que fica público e o que as suas mãos dizem sobre o seu jogo."
    currentHref="/guide/profile" next={{href: '/guide/community', label: 'Comunidade e segurança'}} sections={[
      {
        id: 'seu-perfil', title: 'Seu perfil', summary: 'Uma página só sua, com identidade, mesa, vitrine e saldos.',
        image: {
          src: '/guide/profile-live.webp',
          alt: 'Página Seu perfil com a seção Identidade e, abaixo, os controles de privacidade de Sua vitrine'
        },
        body: <><p>Há dois caminhos, e eles combinam. Toque no seu avatar, no topo: o menu resolve na hora o que é rápido, ou seja, trocar o nome, trocar a foto e escolher o baralho. Ele também mostra se a sua vitrine está pública ou privada, os seus saldos, e leva à Loja, a <b>Seu jogo</b> e a <b>Sair da conta</b>.</p>
          <p>No mesmo menu, <b>Editar perfil</b> abre a página <b>Seu perfil</b>, que tem tudo isso e mais, em seis seções.</p>
          <GuideBullets><li><span><b>Identidade:</b> nome de exibição e foto.</span></li>
            <li><span><b>Sua vitrine:</b> privacidade, conquistas em destaque e ordem das seções.</span></li>
            <li><span><b>Sua mesa:</b> o baralho que você vê e, onde as apostas em dinheiro real estão liberadas, o modo de jogo.</span></li>
            <li><span><b>Filtro de chat:</b> palavras que você mesmo quer que fiquem mascaradas no chat da mesa, além do filtro padrão que já vale para todos.</span></li>
            <li><span><b>Seus saldos:</b> Fichas e, onde as apostas em dinheiro real estão liberadas, Dinheiro real.</span></li>
            <li><span><b>Alertas de carteira:</b> um aviso no saldo de fichas e um limite de compra, ambos opcionais.</span></li></GuideBullets>
          <p>Nome, foto e baralho aparecem nos dois lugares de propósito: é o mesmo controle, salvando no mesmo lugar. Alterou num, o outro já mostra o valor novo.</p></>
      },
      {
        id: 'identidade', title: 'Nome e foto', summary: 'É assim que a mesa, a vitrine e o ranking chamam você.',
        body: <><GuideSteps><li><span>Em <b>Seu perfil</b>, escreva em <b>Nome de exibição</b>, até 40 caracteres, e toque em <b>Salvar nome</b>.</span></li>
          <li><span>Na foto, use a câmera para enviar um JPG ou PNG. <b>Remover foto</b> apaga a imagem.</span></li></GuideSteps>
          <p>No menu do avatar dá para fazer os dois sem sair da tela em que você está: toque no nome para editá-lo ali mesmo, com Enter para salvar e Esc para cancelar, e use a câmera sobre a foto para enviar ou o cesto para apagar.</p>
          <p>Sem foto, o jogo usa suas iniciais. O envio avisa quando termina e quando falha. Trocar de nome não afeta mãos já registradas.</p></>
      },
      {
        id: 'baralho', title: 'Escolher o baralho', summary: 'Preferência visual, aplicada às cartas que você vê.',
        body: <><p>Em <b>Sua mesa</b>, o seletor mostra uma amostra dos quatro ases de cada variante. As gratuitas valem já na próxima mão; as premium aparecem com cadeado e preço, e levam à seção de baralhos da Loja. O mesmo seletor está no menu do avatar, em qualquer tela do jogo.</p>
          <GuideCallout kind="info" title="Somente apresentação">Trocar o baralho não interfere na distribuição, no hash da prova nem nas cartas que você recebe.</GuideCallout></>
      },
      {
        id: 'vitrine', title: 'Montar a vitrine', summary: 'A vitrine começa privada e só mostra o que você habilitar.',
        image: {
          src: '/guide/profile-showcase.webp',
          alt: 'Seção Sua vitrine com a fila de conquistas em destaque, a ordem das seções e os botões Salvar vitrine, Ver como visitante e Copiar link'
        },
        body: <><GuideTerms><GuideTerm term="Vitrine pública">Libera o link da sua vitrine. Enquanto está desligada, o link não abre para ninguém.</GuideTerm>
          <GuideTerm term="Estilo de jogo">Depois de 200 mãos, mostra um rótulo de tendência na mesa e na vitrine. Só pode ser ligado com a vitrine pública.</GuideTerm>
          <GuideTerm term="Mesa visível para amigos">Permite que amigos entrem na sua mesa quando ela é pública. Mesa privada nunca aparece.</GuideTerm>
          <GuideTerm term="Conquistas em destaque">Até três, entre as que já têm progresso. As conquistas passam numa fila de cartas: toque numa para destacá-la e ela vai para o começo, com o selo &quot;Em destaque&quot;. As demais seguem por estrelas. No teclado, as setas percorrem a fila, Home e End vão às pontas, Enter ou Espaço marca e desmarca.</GuideTerm>
          <GuideTerm term="Ordem das seções">As setas para cima e para baixo reordenam Conquistas em Destaque, Melhor Vitória Recente e Cara a Cara, sem arrastar. Melhor Vitória e Cara a Cara também podem ficar fora da vitrine; Conquistas sempre aparece.</GuideTerm></GuideTerms>
          <p>Privacidade, destaques e ordem saem juntos em <b>Salvar vitrine</b>. Nome, foto e baralho não esperam por esse botão: cada um salva sozinho.</p>
          <GuideCallout kind="safe" title="Estatística detalhada não vai junto">Tornar a vitrine pública não publica VPIP, PFR nem 3-bet. Só os rótulos que você autorizou saem do seu perfil.</GuideCallout></>
      },
      {
        id: 'visitante', title: 'Conferir e compartilhar', summary: 'Veja a sua vitrine como ela chega para os outros antes de divulgar.',
        body: <><GuideBullets><li><span><b>Ver como visitante</b> abre a sua vitrine em pré-visualização. Uma faixa dourada, que só você enxerga, avisa disso e devolve você para a edição; tudo abaixo dela é o que a outra pessoa recebe.</span></li>
          <li><span><b>Copiar link</b> aparece depois que a vitrine pública é salva.</span></li></GuideBullets>
          <p>Quem abre a sua vitrine vê nome, foto, seus marcos, os rótulos de estilo autorizados e as seções visíveis na ordem que você escolheu: conquistas em destaque, a sua melhor vitória recente e, se estiver logado, o &quot;Cara a Cara&quot;, ou seja, quantas mãos vocês jogaram juntos, quantas cada um venceu, quantas empataram e o saldo de fichas do confronto.</p>
          <p>Abrindo o seu próprio link, uma faixa avisa que a vitrine é sua e oferece a edição. Com a vitrine privada, você lê o motivo e o caminho para publicá-la; um visitante lê que ela está privada, nunca que o perfil não existe.</p>
          <GuideLink href="/player-profile">Abrir meu perfil</GuideLink></>
      },
      {
        id: 'alertas', title: 'Configurar alertas de carteira', summary: 'Um aviso no perfil, nunca um bloqueio de compra.',
        body: <><p>Em <b>Alertas de carteira</b>, dois limites são opcionais e independentes. Deixe qualquer um em 0 para desligá-lo; <b>Remover alertas</b> aparece assim que algum estiver configurado.</p>
          <GuideTerms><GuideTerm term="Avisar quando o saldo de fichas cair abaixo de">Um aviso aparece em <b>Seus saldos</b> sempre que o seu saldo atual estiver abaixo do valor configurado.</GuideTerm>
            <GuideTerm term="Avisar quando uma compra passar de (R$)">O limite de gasto por compra de fichas, em reais.</GuideTerm></GuideTerms>
          <GuideCallout kind="safe" title="Nunca bloqueia nada">O alerta é só um aviso. Ele não impede uma compra nem muda taxa ou bônus de nenhum pacote.</GuideCallout></>
      },
      {
        id: 'hud', title: 'Ler “Seu jogo”', summary: 'Tendências pré-flop calculadas a partir das suas mãos concluídas.',
        body: <><GuideTerms><GuideTerm term="VPIP">Mãos em que você colocou fichas voluntariamente no pote pré-flop, sem contar os blinds.</GuideTerm>
          <GuideTerm term="PFR">Mãos em que você fez pelo menos um raise pré-flop.</GuideTerm>
          <GuideTerm term="3-bet">Reaumentos diante de um raise, medidos sobre as oportunidades reais.</GuideTerm>
          <GuideTerm term="Radar e badges">Leitura da amostra em participação, iniciativa, pressão, reaumento e seleção. Abra um badge para ver o critério.</GuideTerm></GuideTerms>
          <p>O tamanho da amostra fica visível: com poucas mãos, leia os números como uma primeira impressão. Onde as apostas em dinheiro real estão liberadas, abas separam Fichas de Dinheiro real.</p>
          <GuideCallout kind="safe" title="Estatísticas privadas">A tela “Seu jogo” é visível só para você, em qualquer situação.</GuideCallout></>
      }
    ]}/>;
}
