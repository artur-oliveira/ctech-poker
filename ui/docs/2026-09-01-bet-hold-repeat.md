# Ajuste de aposta por pressionar e segurar (toque e teclado)

**Data:** 2026-09-01 · **Escopo:** `src/lib/hooks/useHoldRepeat.ts`,
`src/components/table/ActionBar.tsx`, `src/lib/betShortcuts.ts`, `src/app/guide/table/page.tsx`

## Problema

No celular, os botões `+` / `−` do seletor de aumento já aceleravam quando o jogador segurava o
toque. No desktop, `ArrowLeft` / `ArrowRight` davam **um passo por tecla pressionada**: chegar a um
total alto exigia dezenas de toques, porque `isBetAdjustKey` descarta `event.repeat` (o
auto-repeat do sistema operacional) e nada assumia a cadência no lugar dele.

## Solução

A repetição por pressão saiu de dentro de `BetStepButton` e virou `useHoldRepeat`, usado agora
pelos dois caminhos de entrada:

- **Ponteiro:** `onPointerDown` inicia; `pointerup` / `pointercancel` / `pointerleave` param. O
  clique final é engolido quando a pressão já repetiu (`consumeRepeated`).
- **Teclado:** o primeiro `keydown` de `ArrowLeft` / `ArrowRight` aplica um passo imediato e inicia
  o mesmo hold; `keyup` e `blur` da janela param. Os `keydown` de auto-repeat do sistema continuam
  ignorados — a cadência é nossa, então acelera igual em qualquer teclado ou sistema.

Cadência compartilhada: `HOLD_DELAY_MS` 420 ms até o primeiro passo automático, `HOLD_REPEAT_MS`
130 ms entre eles, com a rampa `1 → 5 → 10` passos (ticks < 5, < 11, acima). Com `Ctrl`, o passo
base é multiplicado por `FAST_STEP_STRIDE` (3), inclusive durante o hold.

## Detalhes que não são óbvios

- O efeito de teclado de `RaiseControl` roda de novo a cada mudança de valor (`safeAmount` está nas
  dependências). Por isso o cleanup **não** chama `hold.stop()`: pararia o hold no próprio primeiro
  passo. O hold termina por `keyup`, `blur`, `inactive` ou desmontagem (efeito do próprio hook).
- `blur` da janela é tratado porque alt-tab engole o `keyup` e o valor continuaria subindo sozinho.
- `betShortcutAmount` perdeu o parâmetro `fast`: quem quer passo triplo multiplica `step` por
  `FAST_STEP_STRIDE`, o que também é como o hold aplica a rampa. Uma regra de clamp só, um caminho.

## Acessibilidade e documentação

- O label do slider (`sr-only`) e o guia da mesa (`/guide/table`) passaram a citar as setas e o
  segurar-para-acelerar; `aria-keyshortcuts` já listava as setas.
- Testes em `ActionBar.test.tsx`: aceleração ao segurar a seta, parada no `keyup`, passo triplo com
  `Ctrl` e parada no `blur`, além dos testes de hold por ponteiro que já existiam.
