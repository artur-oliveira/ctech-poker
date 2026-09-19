'use client';

import {ShoppingBag} from 'lucide-react';
import {GuideBullets, GuideCallout, GuideLink, GuidePage, GuideSteps, GuideTerm, GuideTerms} from '@/components/guide/GuidePage';

export default function StoreGuide() {
  return <GuidePage icon={ShoppingBag} eyebrow="LOJA" title="Reações, cosméticos e fichas"
    description="Reações premium, baralhos, feltros, recompensa diária, pacotes via Pix e o histórico de tudo o que você liberou."
    currentHref="/guide/store" next={{href: '/guide/profile', label: 'Configurar meu perfil'}} sections={[
      {
        id: 'secoes', title: 'Os seis departamentos', summary: 'O diretório no topo leva direto à seção certa.',
        image: {src: '/guide/store-live.webp', alt: 'Loja com o diretório de reações, baralhos, feltro, combos, fichas e compras'},
        body: <><p>Abra a <b>Loja</b> na navegação ou toque no saldo ao lado do avatar. Cada entrada do diretório mostra quantos itens você já liberou; a de fichas mostra o saldo atual.</p>
          <GuideBullets><li><span><b>Reações premium:</b> seis gestos além dos gratuitos.</span></li>
            <li><span><b>Baralhos:</b> variações de cor e desenho das cartas.</span></li>
            <li><span><b>Feltro:</b> os temas de mesa além do Clássico.</span></li>
            <li><span><b>Combos:</b> baralho e feltro salvos juntos, com nome, para aplicar os dois de uma vez.</span></li>
            <li><span><b>Fichas:</b> saldo, recompensa diária e pacotes.</span></li>
            <li><span><b>Compras e estornos:</b> recibos de reações, baralhos e feltros. As compras de fichas ficam na própria seção de fichas.</span></li></GuideBullets>
          <GuideCallout kind="safe" title="Fichas não são dinheiro">O saldo de fichas serve para buy-ins, não pode ser sacado e não vira moeda real.</GuideCallout></>
      },
      {
        id: 'premium', title: 'Liberar uma reação, um baralho ou um feltro', summary: 'Uma compra, uso permanente, sem assinatura nem consumo por uso.',
        body: <><p>O topo do diálogo mostra o item junto do que você já usa do outro tipo: um baralho combinado com o seu
          feltro atual, ou um feltro combinado com o seu baralho atual.</p>
          <GuideSteps><li><span>Escolha o item. O cartão mostra o preço em fichas e o preço em reais.</span></li>
          <li><span>Pague com <b>fichas</b>, com confirmação imediata, ou com <b>Pix</b>, pelo QR code do passo seguinte.</span></li>
          <li><span>Confirmado, o item já vale em qualquer mesa: a reação entra no painel, o baralho aparece em <b>Seu perfil</b>, na seção <b>Sua mesa</b>, e o feltro nas preferências da mesa.</span></li></GuideSteps>
          <p>Reações premium bloqueadas continuam visíveis na mesa com um cadeado; tocar nelas abre a compra sem tirar você da partida. Até três reações, premium ou não, podem virar atalho fixo ao lado do botão.</p>
          <GuideCallout kind="info" title="Estorno enquanto o item não foi usado">Um cosmético só pode ser estornado se nunca tiver sido selecionado. O servidor verifica isso antes de autorizar, e o item volta a ficar bloqueado.</GuideCallout></>
      },
      {
        id: 'combos', title: 'Salvar e aplicar um combo', summary: 'Baralho e feltro salvos juntos, com um nome, para trocar os dois em um só toque.',
        body: <><p>Na seção <b>Combos</b>, o card <b>Combinação atual</b> mostra o baralho e o feltro que você usa agora. Toque em <b>Salvar como combo</b>, dê um nome e ele entra na sua lista.</p>
          <GuideSteps><li><span>Em cada combo salvo, toque em <b>Aplicar</b>. Um diálogo mostra o baralho e o feltro do combo antes de qualquer troca.</span></li>
          <li><span>Confirme em <b>Aplicar combo</b>: os dois itens passam a valer na próxima mão, em qualquer mesa.</span></li>
          <li><span><b>Excluir</b> remove o combo da lista; os itens salvos nele continuam seus.</span></li></GuideSteps>
          <p>Até cinco combos por jogador. No limite, salve um novo só depois de excluir outro.</p>
          <GuideCallout kind="info" title="Sempre reverifica a posse">Se um item premium do combo tiver sido estornado depois de salvo, aplicar o combo é recusado e nada muda. Nunca aplica um item que você não possui mais.</GuideCallout></>
      },
      {
        id: 'diaria', title: 'Resgatar a recompensa diária', summary: 'Um resgate por ciclo, direto no seu saldo.',
        body: <><GuideSteps><li><span>Um ponto dourado na Loja avisa quando a recompensa está pronta. Nada é resgatado automaticamente.</span></li>
          <li><span>Use <b>Resgatar fichas grátis</b> na seção de fichas. O valor só é revelado no resgate.</span></li>
          <li><span>Depois, o painel recua para uma linha com o valor recebido e o tempo até a próxima.</span></li></GuideSteps>
          <p>Recarregar a página não antecipa o ciclo: a disponibilidade vem do serviço. Se a consulta falhar, o painel mantém uma nova tentativa. Sem fichas para continuar em uma mesa, a recompra também oferece esse resgate.</p></>
      },
      {
        id: 'pix', title: 'Comprar um pacote de fichas', summary: 'O pagamento adiciona fichas, nunca saldo sacável.',
        body: <><GuideSteps><li><span>Compare total, fichas base, bônus e preço. Os pacotes aparecem em ordem crescente de preço.</span></li>
          <li><span>No diálogo, escaneie o QR code ou copie o código Pix pelo ícone ao lado do campo.</span></li>
          <li><span>Conclua no aplicativo do banco antes da expiração indicada.</span></li>
          <li><span>Você pode fechar o diálogo: a confirmação continua sendo verificada e chega em tempo real. <b>Verificar pagamento</b> força uma checagem.</span></li></GuideSteps>
          <GuideCallout kind="warning" title="Confira antes de pagar">Valide recebedor e valor no seu banco antes de concluir.</GuideCallout></>
      },
      {
        id: 'promocional', title: 'Código promocional e pacote de boas-vindas', summary: 'Um código digitado define o pacote; o pacote de boas-vindas vale uma vez por conta.',
        body: <><p>Abaixo dos pacotes, o campo <b>Código promocional</b> é opcional. Digite o código e toque em <b>Aplicar código</b>: o pacote e o preço são definidos pelo próprio código, não pelo que estava selecionado antes. Um código já usado, vencido ou digitado errado mostra o motivo ali mesmo, sem abrir cobrança nenhuma.</p>
          <p>O <b>pacote de boas-vindas</b> aparece com um selo dourado na grade de pacotes e vale uma única vez por conta. Depois de resgatado, o cartão mostra “Já resgatado” e fica desabilitado.</p>
          <GuideCallout kind="safe" title="Pagamento sempre pelo Pix">Um código promocional nunca pula o pagamento: ele só muda qual pacote e qual preço você paga.</GuideCallout></>
      },
      {
        id: 'status', title: 'Os estados de uma compra', summary: 'O histórico separa cobrança aberta de compra concluída.',
        body: <GuideTerms><GuideTerm term="Aguardando pagamento">O Pix ainda vale. <b>Continuar pagamento</b> reabre QR e código.</GuideTerm>
          <GuideTerm term="Confirmada">O pagamento foi reconhecido e o crédito entrou no saldo.</GuideTerm>
          <GuideTerm term="Expirada">O prazo terminou. <b>Gerar novo Pix para este pacote</b> cria outra cobrança sem refazer a escolha.</GuideTerm>
          <GuideTerm term="Falhou">A tentativa não foi concluída e nada é creditado.</GuideTerm>
          <GuideTerm term="Estornada">Uma compra confirmada foi revertida e o crédito correspondente saiu do saldo.</GuideTerm></GuideTerms>
      },
      {
        id: 'estornos', title: 'Retomar, conferir e estornar', summary: 'Cada tipo de compra tem seu recibo e sua própria regra de estorno.',
        body: <><GuideBullets><li><span>Compare data, quantidade, preço e status antes de agir.</span></li>
          <li><span>Em fichas, <b>Solicitar estorno</b> mostra o valor em reais, as fichas removidas e o saldo projetado. O servidor recusa se qualquer ficha daquele crédito já tiver sido usada.</span></li>
          <li><span>Em cosméticos, o estorno devolve fichas ao saldo ou volta pela mesma compra Pix, conforme o meio usado.</span></li>
          <li><span>Uma falha de catálogo ou de histórico mostra a tentativa manual só naquela parte, sem esconder o resto da Loja.</span></li></GuideBullets>
          <GuideCallout kind="safe" title="Estorno não converte fichas em dinheiro">Ele apenas reverte a compra. Nenhuma operação aqui movimenta dinheiro real.</GuideCallout>
          <GuideLink href="/store">Abrir a Loja</GuideLink></>
      }
    ]}/>;
}
