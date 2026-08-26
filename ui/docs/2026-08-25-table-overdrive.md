# Mesa em overdrive — 2026-08-25

## Objetivo

Transformar os recursos já existentes da mesa em uma experiência coordenada de “mesa viva”, sem alterar regras de poker
nem inventar estado ausente do protocolo. A direção preserva a identidade de feltro, papel, madeira e ouro e mantém a
mesa como uma SPA estática, inteiramente cliente.

## Experiência entregue

- Cada mudança real de street recria uma única camada de luz sobre o feltro. A camada é informativa, não fica em loop e
  desaparece imediatamente quando `prefers-reduced-motion: reduce` está ativo.
- O board de run it twice ganha uma identificação explícita — “Rodando duas vezes” e “Dois boards no mesmo all-in” —
  antes das cartas comuns e das duas distribuições.
- O resultado da mão continua distinguindo vitória, derrota, empate, fold e resultado misto, mas agora também mostra um
  acerto completo quando existem side pots ou dois runouts. Cada linha informa pote principal/lateral, board quando
  publicado, valor, vencedor, divisão, total distribuído, parcela individual quando publicada e se o jogador não
  disputou aquele pote.
- O acerto usa somente `pot_results`, `winner_player_ids`, `eligible_player_ids`, `payout_amount`, `refund` e `runout`
  recebidos pelo socket. Não reconstrói cartas fechadas, elegibilidade ou vencedores.
- O seletor de aumento mostra uma régua visual contínua entre o valor escolhido e o máximo permitido. O texto e o
  `aria-valuetext` existentes continuam sendo a fonte semântica; a régua é apenas reforço visual.
- Sons de cartas, fichas e alerta de turno agora têm um controle mestre “Sons da mesa”. Ele começa desligado e é
  persistido em `ctech-poker:table-preferences:v1`; política de autoplay do navegador não é tratada como consentimento.
  “Dealer auditivo” permanece uma preferência separada para narração.

## Responsividade e acessibilidade

- O resultado tem altura máxima relativa a `dvh`, rolagem interna e `overscroll-behavior: contain`, preservando a regra
  de uma única viewport da mesa em retrato.
- As linhas de pote mantêm alvo/altura mínima de 44 px e valores tabulares. Cor nunca é a única indicação: vencedor,
  divisão, devolução e inelegibilidade aparecem em texto.
- O foco do resultado é removível e minimizável como antes. O live region continua anunciando o desfecho textual.
- Street wash, spotlight e transição da régua respeitam movimento reduzido; o estado final permanece legível.

## Cenas de verificação

- `scenario=side_pot`: acerto de pote principal e lateral, incluindo pote que o viewer não disputou.
- `scenario=run_it_twice`: identificação dos dois boards e selo correspondente no resultado.
- `scenario=full_hand`: aposta compacta, card peek, deal/reveal e transições de street.
- Breakpoints visuais conferidos em `1440×900`, `390×844` e `320×568` usando `npm run dev:mock`.
