'use client';

import {ShoppingBag} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function StoreGuide() {
  return <GuidePage icon={ShoppingBag} eyebrow="FICHAS SANDBOX" title="Recompensas e compras na Loja"
    description="A Loja reúne a recompensa gratuita, pacotes pagos via Pix e o histórico de cada tentativa de compra."
    currentHref="/guide/store" next={{href: '/guide/profile', label: 'Configurar meu perfil'}} sections={[
      {
        id: 'acesso', title: 'Saldo e indicação de recompensa', summary: 'A Loja é o destino único para ganhar ou adicionar fichas sandbox.',
        image: {src: '/guide/store-live.webp', alt: 'Loja com saldo sandbox, recompensa diária e aba de compras via Pix'},
        body: <><p>Abra <b>Loja</b> na navegação ou toque no saldo ao lado do avatar. Quando a recompensa diária está pronta, um ponto dourado destaca a Loja sem resgatar nada automaticamente.</p>
          <GuideCallout kind="safe" title="Fichas não são dinheiro">Todo saldo descrito aqui é sandbox: serve para buy-ins do jogo, não pode ser sacado e não representa moeda real.</GuideCallout></>
      },
      {
        id: 'diaria', title: 'Resgatar a recompensa diária', summary: 'Um resgate por ciclo adiciona fichas diretamente ao saldo sandbox.',
        body: <><GuideSteps><li><span>Abra a aba <b>Ganhar grátis</b>.</span></li>
          <li><span>Quando disponível, escolha <b>Receber fichas</b>.</span></li>
          <li><span>A animação revela o valor e o saldo é atualizado.</span></li>
          <li><span>O painel passa a mostrar o tempo restante até a próxima oportunidade.</span></li></GuideSteps>
          <p>Se a consulta falhar, o painel mantém uma ação para tentar novamente. Recarregar a página não antecipa o próximo ciclo, pois a disponibilidade vem do serviço.</p></>
      },
      {
        id: 'pix', title: 'Comprar um pacote via Pix', summary: 'O pagamento adiciona fichas sandbox, nunca saldo sacável.',
        body: <><GuideSteps><li><span>Abra <b>Comprar via Pix</b> e escolha o total de fichas e preço desejados.</span></li>
          <li><span>No diálogo, escaneie o QR code ou copie o código Pix discreto pelo ícone ao lado do campo.</span></li>
          <li><span>Conclua no aplicativo do banco antes da expiração indicada.</span></li>
          <li><span>Mantenha o diálogo aberto ou volte depois: a confirmação é consultada periodicamente e também chega em tempo real quando disponível.</span></li></GuideSteps>
          <GuideCallout kind="warning" title="Confira antes de pagar">Valide recebedor e valor no seu banco. Um QR de ambiente mock é apenas visual e não deve ser tratado como cobrança real.</GuideCallout></>
      },
      {
        id: 'status', title: 'Entender os estados da compra', summary: 'O histórico diferencia uma cobrança aberta de uma compra concluída.',
        body: <GuideTerms><GuideTerm term="Aguardando pagamento">O Pix ainda está válido. Use <b>Continuar pagamento</b> para reabrir QR e código.</GuideTerm>
          <GuideTerm term="Confirmada">O pagamento foi reconhecido e as fichas foram adicionadas ao saldo.</GuideTerm>
          <GuideTerm term="Expirada">O prazo terminou; o código fica desabilitado. Inicie outra compra se ainda quiser o pacote.</GuideTerm>
          <GuideTerm term="Falhou">A tentativa não foi concluída. Nenhuma ficha deve ser creditada.</GuideTerm>
          <GuideTerm term="Estornada">Uma compra confirmada foi revertida e as fichas correspondentes foram retiradas.</GuideTerm></GuideTerms>
      },
      {
        id: 'historico', title: 'Retomar, conferir e estornar', summary: 'Compras recentes permanecem na mesma aba dos pacotes.',
        body: <><GuideBullets><li><span>Use <b>Continuar pagamento</b> em compras pendentes.</span></li>
          <li><span>Compare data, total de fichas, preço e status antes de tomar uma ação.</span></li>
          <li><span>Quando o ambiente oferecer estorno, a ação aparece somente para compras confirmadas elegíveis.</span></li>
          <li><span>Falhas de catálogo ou histórico exibem uma tentativa manual sem esconder as demais partes da Loja.</span></li></GuideBullets>
          <GuideLink href="/store">Abrir a Loja</GuideLink></>
      }
    ]}/>;
}
