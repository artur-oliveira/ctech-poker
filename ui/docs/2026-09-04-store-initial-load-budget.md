# /store: orçamento de carga inicial (#206)

## O problema

Montar `/store` disparava **9 requests** antes de qualquer interação: perfil, SKUs sandbox,
catálogo de reações, catálogo de baralhos, catálogo de feltros e a **primeira página de quatro
históricos de compra** (fichas, reações, baralhos, feltros).

## O orçamento agora

| carga | requests |
| --- | --- |
| inicial | perfil + 4 catálogos = **5** |
| por departamento alcançado | o histórico daquele departamento |

Os catálogos continuam ansiosos de propósito: eles respondem os contadores "N de M liberadas" da
navegação da loja (visíveis no topo) e são a **única** fonte de posse (`entry.owned`). O histórico
não é a página — ele só acrescenta recibos, o botão "Acompanhar" de um Pix pendente e o "Estornar"
de um item já liberado.

Cada histórico é armado pela sua própria seção entrar em vista, via `lib/hooks/useInViewOnce.ts`
(latch de mão única, `rootMargin` de 400px, igual em espírito aos latches de
`useTableProgressiveSession`). A lista "Compras e estornos" arma os três históricos de itens de uma
vez, porque é isso que ela renderiza.

`useInViewOnce` começa **armado** onde não existe `IntersectionObserver` (SSR, jsdom): a ausência do
observer pode custar um request a mais, nunca esconder conteúdo atrás de um latch que ninguém
consegue abrir.

## Por que um histórico adiado se declara `isLoading`

Porque é a verdade — as linhas ainda não chegaram — e mantém o estado monotônico: esqueleto e
depois a grade. Reportá-lo como carregado pintaria a grade do departamento **sem** as ações de
retomar/estornar e a jogaria de volta para o esqueleto no instante em que o histórico armasse; pior,
um Pix pendente apareceria como "Liberar" e convidaria a uma segunda compra.

## O que não mudou

- **A invalidação continua na raiz `['wallet']`.** Nomear um subconjunto foi exatamente o que já
  deixou a loja mostrando posse que não existia mais (veja o comentário em `useLobbyRealtime.ts`).
  Como uma query desabilitada não refaz fetch, a invalidação já não recarrega os históricos que o
  jogador não alcançou.
- **Não há endpoint bootstrap de catálogo + posse.** Isso é mudança de backend; a issue #206 é de
  frontend e o fan-out de catálogo permanece em quatro requests.

## Como validar

`src/app/(app)/store/page.test.tsx` — "defers every purchase history until its own department comes
into view" traz o próprio stub de `IntersectionObserver` e afirma os quatro `enabled: false`
iniciais, o arme por seção e o arme conjunto da lista de atividades; "keeps a department on screen
when only its history fails" cobre a falha isolada. `src/lib/hooks/useInViewOnce.test.tsx` cobre o
latch (sem observer, com observer, unmount).
