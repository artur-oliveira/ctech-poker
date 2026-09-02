'use client';

import {Compass} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function BasicsGuide() {
  return <GuidePage icon={Compass} eyebrow="COMECE AQUI" title="Do lobby à primeira mão"
    description="Como escolher uma mesa, quantas fichas levar e quando a próxima mão começa."
    currentHref="/guide/basics" next={{href: '/guide/table', label: 'Conhecer a mesa'}} sections={[
      {
        id: 'conta', title: 'Entrar e aceitar os termos', summary: 'Saldo, mãos, conquistas e preferências ficam ligados à sua conta CTech.',
        body: <><GuideSteps><li><span>Na página inicial, use <b>Jogar agora</b> e conclua o acesso pela CTech Account.</span></li>
          <li><span>No primeiro acesso ao poker, abra os Termos e a Política de Privacidade, marque o aceite e confirme.</span></li>
          <li><span>Seu perfil é criado com o nome da conta. Você troca o nome e a foto quando quiser.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="O aceite é explícito">A caixa não vem marcada e o jogo não abre sem ela. Se o registro do aceite falhar, a tela oferece uma nova tentativa em vez de deixar você entrar pela metade.</GuideCallout>
          <GuideCallout kind="info" title="Se o acesso não completar">A tela distingue o motivo em vez de mostrar sempre a mesma mensagem: uma instabilidade passageira oferece <b>Tentar novamente</b> tentando de novo na hora, sem pedir um novo login; um código de acesso expirado ou já usado pede para entrar de novo; e uma indisponibilidade do provedor de login leva à tela de manutenção.</GuideCallout></>
      },
      {
        id: 'lobby', title: 'O lobby em duas escolhas', summary: 'Primeiro os blinds, depois o tamanho da mesa.',
        image: {src: '/guide/lobby.webp', alt: 'Lobby com a lista de blinds e os três formatos de mesa'},
        body: <><p>Escolha um par de blinds e em seguida o formato: <b>Heads-up</b> (2), <b>6-max</b> (6) ou <b>Full-ring</b> (9). Cada cartão mostra quantas mesas daquele formato têm vaga agora e a faixa de entrada correspondente.</p>
          <GuideTerms><GuideTerm term="Blinds">Apostas obrigatórias que abrem cada mão. O primeiro número é o small blind; o segundo, o big blind.</GuideTerm>
            <GuideTerm term="Entrar agora">Existe mesa pública com vaga naquele formato e você senta nela.</GuideTerm>
            <GuideTerm term="Criar mesa">Não há vaga; uma nova mesa pública é aberta com a sua escolha.</GuideTerm>
            <GuideTerm term="Sandbox">Fichas fictícias. Servem para jogar, não têm saque nem conversão em dinheiro.</GuideTerm></GuideTerms>
          <p>Se você já está sentado em algum lugar, <b>Sua mesa continua aberta</b> aparece no topo com a entrada usada e leva de volta ao mesmo assento — nunca cria um segundo. Um ponto dourado na Loja significa que a recompensa diária já pode ser resgatada.</p></>
      },
      {
        id: 'buyin', title: 'Buy-in e auto rebuy', summary: 'O buy-in transfere fichas da carteira para a sua pilha na mesa.',
        image: {src: '/guide/buyin.webp', alt: 'Tela de buy-in com controle deslizante, faixa permitida e opção de auto rebuy'},
        body: <><p>A faixa aceita vai de 20 a 100 big blinds. Ajuste o valor no controle deslizante e confirme; nada é debitado antes disso. Ao sair, as fichas que sobraram voltam para a mesma carteira.</p>
          <GuideBullets><li><span><b>Auto rebuy:</b> se a sua pilha zerar, a mesa recompra automaticamente o mesmo valor e você continua jogando sem parar a sessão. Só existe em mesas sandbox e pode ser ligado aqui ou depois, na recompra.</span></li>
            <li><span><b>Se uma mão já começou:</b> você senta na hora e entra a partir da próxima distribuição.</span></li></GuideBullets>
          <GuideCallout kind="warning" title="Saldo insuficiente">Se o saldo não cobre o buy-in mínimo, resgate a recompensa diária na Loja ou escolha blinds menores.</GuideCallout></>
      },
      {
        id: 'privada', title: 'Criar uma mesa privada', summary: 'Salas privadas não aparecem na lista pública: o link é o acesso.',
        image: {src: '/guide/create-room.webp', alt: 'Diálogo de mesa privada com modo, stakes, lugares e a opção de rodar duas vezes'},
        body: <><GuideSteps><li><span>No lobby, escolha <b>Mesa privada</b>.</span></li>
          <li><span>Defina o modo, a stake e quantos lugares a mesa terá.</span></li>
          <li><span>Decida se a sala vai <b>permitir rodar duas vezes</b>. Isso libera o recurso; cada jogador ainda ativa por conta própria nas preferências.</span></li>
          <li><span>Crie a sala e compartilhe o link — o código de convite já vai dentro dele. Na mesa, <b>Convidar</b> também chama amigos direto da sua lista.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="O link é a chave">Qualquer pessoa com o link pode tentar entrar. Trate um convite privado como senha e não publique em lugar aberto.</GuideCallout></>
      },
      {
        id: 'primeira-mao', title: 'Quando a mão começa', summary: 'A mesa espera jogadores suficientes e depois anuncia a próxima distribuição.',
        body: <><GuideBullets><li><span><b>Aguardando jogadores:</b> ainda não há gente suficiente para distribuir.</span></li>
          <li><span><b>Próxima mão:</b> um contador no feltro marca quando a rodada seguinte começa.</span></li>
          <li><span><b>Sentar fora:</b> pausa sua participação sem soltar o assento; a pilha continua sua.</span></li>
          <li><span><b>Voltar a jogar:</b> devolve você à próxima mão, pagando o big blind quando for o caso.</span></li></GuideBullets>
          <GuideLink href="/poker-rules">Rever as regras e o ranking das mãos</GuideLink></>
      }
    ]}/>;
}
