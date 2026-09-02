# Guia reescrito contra o produto atual — 2026-09-01

O guia (`src/app/guide`) tinha sido escrito antes de várias entregas (reações premium,
cosméticos na Loja, treinador, espiar cartas, ação preparada de all-in, saída no meio da mão,
convites diretos, presença entre amigos) e descrevia telas que não existem mais. Esta passagem
auditou `src/app` inteiro e reescreveu o guia a partir da implementação.

## O que estava errado

- **Loja:** o guia falava em três seções (reações, fichas, compras). A Loja tem cinco
  departamentos — reações premium, baralhos, feltro, fichas sandbox, compras e estornos — e
  qualquer item premium pode ser pago com **fichas ou Pix**.
- **Preferências da mesa:** faltavam sons da mesa (desligado por padrão) e o treinador; os temas
  de feltro deixaram de ser preferência local e viraram cosmético comprável, salvo no perfil.
- **Perfil:** o menu não tem "Créditos e recompensas"; tem modo de jogo, baralho (com premium
  bloqueado levando à Loja), saldos, Seu jogo e Vitrine. A vitrine ganhou **Mesa visível para
  amigos**, e o perfil público mostra melhor vitória recente e o confronto direto.
- **Mãos:** os filtros de resultado não existem mais (só abas de carteira + indicadores do que já
  foi carregado). O compartilhamento agora escolhe **brag/bad beat**, se as cartas entram e a
  validade do link (24 h, 7 dias, 30 dias).
- **Mesa:** não havia nada sobre espiar cartas (1/2, com chance e combinação represadas até as
  duas cartas), atalhos de teclado, menu "Mais ações da mesa", maior pote do dia, pedir a mão do
  vencedor, saída no meio da mão com cancelamento, resumo de sessão, auto rebuy ou favoritas de
  reação.
- **Lobby:** a escolha virou dois passos (blinds e depois heads-up / 6-max / full-ring) e a
  entrada é 20–100 BB; a mesa privada oferece modo, stakes, lugares e "permitir rodar duas vezes".
- **Comunidade:** Pessoas tem cinco abas (amigos, solicitações, recentes, bloqueados,
  atividades), o convite de mesa também sai da lista de amigos e a denúncia tem categorias.

## O que mudou no guia

- Sete tópicos mantidos; seções reorganizadas. `basics` perdeu a seção duplicada de mesa pública,
  `table` passou a ter atalhos, cartas e "pausar, recomprar e sair" próprios, `store` foi
  reescrito em torno dos cinco departamentos.
- Copy encurtada: uma ideia por frase, sem narrar a interface.
- `GuideTerms` aceita `variant="keys"`, que imprime a coluna de termos na fonte mono usada pelos
  chips `<kbd>` da barra de ações (`.guide-keys` em `globals.css`, sem token novo).

## Capturas

`scripts/capture-og-images.mjs` ganhou quatro alvos de guia — `table-reactions`,
`table-preferences`, `hand-replay` e `people-live` — com três novas ações de preparo
(`open-reactions`, `open-preferences`, `advance-replay`). Todas as capturas do guia foram
regeradas com `npm run og:capture -- --guide` contra o servidor mock (`npm run dev:mock`).
