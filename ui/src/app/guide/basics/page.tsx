'use client';

import {Compass} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function BasicsGuide() {
  return <GuidePage icon={Compass} eyebrow="COMECE AQUI" title="Do lobby à primeira mão"
    description="O caminho mais curto para encontrar uma mesa, escolher seu buy-in e entender quando a partida começa."
    currentHref="/guide/basics" next={{href: '/guide/table', label: 'Conhecer a mesa'}} sections={[
      {
        id: 'conta', title: 'Entrar e confirmar os termos', summary: 'Mesas, preferências e histórico ficam ligados à sua conta CTech.',
        body: <><GuideSteps><li><span>Na página inicial, escolha <b>Jogar agora</b> ou <b>Entrar</b> e conclua o acesso pela CTech Account.</span></li>
          <li><span>No primeiro acesso ao poker, abra Termos e Política de Privacidade, marque o aceite e confirme.</span></li>
          <li><span>O sistema cria seu perfil e pode aproveitar o nome da conta como nome de exibição inicial.</span></li>
          <li><span>Depois disso, você chega ao lobby; o mesmo login protege saldo, mãos, conquistas e preferências de perfil.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="Aceite único e explícito">A caixa não vem marcada. Se o perfil ou o registro do aceite falhar, a tela mantém uma ação para tentar novamente sem entrar em uma mesa parcialmente autenticada.</GuideCallout></>
      },
      {
        id: 'lobby', title: 'Entenda o lobby', summary: 'Cada opção representa uma faixa de blinds e as mesas disponíveis nela.',
        image: {src: '/guide/lobby.webp', alt: 'Lobby do CTech Poker com mesas organizadas por blinds'},
        body: <><p>O lobby é o ponto de partida depois do login. O saldo no topo leva à Loja; a navegação abre Guia, Ranking, Conquistas e Mãos. Se você já está sentado, o aviso <b>Voltar à mesa</b> recupera a partida ativa sem criar outro assento.</p>
          <GuideTerms><GuideTerm term="Blinds">Apostas obrigatórias que iniciam cada mão. A mesa mostra o par small blind / big blind, por exemplo 10 / 20.</GuideTerm>
            <GuideTerm term="Lugares">Quantidade máxima de jogadores. A ocupação atual aparece na opção da mesa.</GuideTerm>
            <GuideTerm term="Sandbox">Modo de treino com fichas fictícias, sem saque e sem valor monetário.</GuideTerm></GuideTerms></>
      },
      {
        id: 'publica', title: 'Entrar em uma mesa pública', summary: 'Escolha a stake; o sistema encontra uma sala compatível.',
        body: <><GuideSteps><li><span>Confira seu saldo sandbox e escolha blinds proporcionais ao valor que pretende levar.</span></li>
          <li><span>Ative a opção da stake. Enquanto a entrada é preparada, os demais botões ficam protegidos contra cliques duplicados.</span></li>
          <li><span>Na tela de buy-in, ajuste o valor dentro do mínimo e máximo permitidos e confirme.</span></li>
          <li><span>Você entra sentado. Se uma mão já estiver em andamento, participa a partir da próxima.</span></li></GuideSteps>
          <GuideCallout kind="tip" title="Escolha confortável">Para aprender os controles, prefira uma stake que permita várias mãos sem consumir grande parte do saldo em um único buy-in.</GuideCallout></>
      },
      {
        id: 'buyin', title: 'Confirmar o buy-in', summary: 'O buy-in transfere fichas da sua carteira sandbox para a pilha da mesa.',
        image: {src: '/guide/buyin.webp', alt: 'Painel de buy-in com faixa de fichas e botão para sentar'},
        body: <><p>O painel informa blinds, faixa aceita e saldo disponível. Nada é debitado antes da confirmação. Ao sair da mesa, as fichas restantes da sua pilha voltam para a carteira correspondente.</p>
          <GuideCallout kind="warning" title="Saldo insuficiente">Se o saldo não atingir o buy-in mínimo, visite a Loja para resgatar a recompensa diária ou obter fichas sandbox.</GuideCallout></>
      },
      {
        id: 'privada', title: 'Criar ou abrir uma mesa privada', summary: 'Salas privadas usam um link de convite e não aparecem como entrada pública comum.',
        image: {src: '/guide/create-room.webp', alt: 'Diálogo para configurar uma mesa privada'},
        body: <><GuideSteps><li><span>No lobby, escolha <b>Mesa privada</b>.</span></li>
          <li><span>Defina a stake e a quantidade de assentos oferecida pela configuração.</span></li>
          <li><span>Crie a sala e compartilhe o link. O código de convite já vai dentro dele.</span></li>
          <li><span>Quem recebe o link ainda confirma o próprio buy-in antes de sentar.</span></li></GuideSteps>
          <GuideCallout kind="safe" title="O link controla o acesso">Trate o convite como uma chave: qualquer pessoa com o link pode tentar abrir a sala. Não publique um link que deveria ficar entre amigos.</GuideCallout></>
      },
      {
        id: 'primeira-mao', title: 'Quando a mão começa', summary: 'A mesa pode aguardar mais jogadores ou uma contagem para a próxima distribuição.',
        body: <><GuideBullets><li><span><b>Aguardando jogadores:</b> ainda não há participantes suficientes para distribuir.</span></li>
          <li><span><b>Próxima mão:</b> o contador indica quando a nova rodada começa.</span></li>
          <li><span><b>Sentar fora:</b> pausa sua participação sem encerrar imediatamente o assento.</span></li>
          <li><span><b>Voltar a jogar:</b> torna você elegível para a próxima mão.</span></li></GuideBullets>
          <GuideLink href="/poker-rules">Rever as regras e o ranking das mãos</GuideLink></>
      }
    ]}/>;
}
