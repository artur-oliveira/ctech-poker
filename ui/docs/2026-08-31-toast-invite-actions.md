# 2026-08-31 — Polimento dos botões de convite no toast

## Contexto

O toast de aviso (`Notifier` / `.api-toast`) usa `<button>` cru tanto para o botão de
fechar quanto para as ações opcionais (`AppNotification.actions`). O `table_invite`
recebido em tempo real (`useLobbyRealtime`) renderiza duas ações: **Entrar** e **Recusar**.

## Problema

A regra `.api-toast button` (destinada só ao botão de fechar) atingia também os botões
de ação:

- `width: 26px; height: 26px; display: grid` recortava o rótulo — só o quadrado de 26px
  era clicável, então clicar no texto "Entrar"/"Recusar" visível não fazia nada;
- `background: transparent` + `.api-toast button:hover { background: var(--black-08) }`
  produzia um hover escuro estranho sobre o papel claro do toast;
- `.api-toast-actions button` só conseguia sobrescrever `width/height/border`, não
  `display`, `flex` nem o hover.

## Correção (`src/app/globals.css`)

- Regra do botão de fechar restrita a `.api-toast > button` (filho direto), com hover
  trocado por um leve `color-mix` do `--toast-ink` em vez de `--black-08`.
- `.api-toast-actions button` agora é um botão compacto real: `inline-flex`,
  `min-height: 36px`, `padding: 0 14px`, raio `--rounded-control`, hairline em
  `--toast-ink`, hover que muda superfície/borda e `:active` com `translateY(1px)`.
- `.api-toast-actions button:first-child` é preenchido com `--brand` / texto
  `--on-brand` (hover `--brand-bright`) — a ação primária do grupo (Entrar, ou a única
  ação nos demais toasts). Sem novos tokens.

Sem mudança de comportamento, markup ou cópia. Testes de `Notifier` e
`socialComponents` seguem passando.
