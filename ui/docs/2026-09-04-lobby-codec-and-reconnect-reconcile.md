# Codec de mesa separado do gateway, e reconciliação de reconnect (#228)

## O problema

`RealtimeBridge` é montado pelo layout do grupo `(app)`, ou seja: **toda** rota autenticada
(`/lobby`, `/store`, `/profile`, `/hands`, `/achievements`, `/people`, `/leaderboard`) monta
`useLobbyRealtime`. Ele decodificava os frames com `lib/ws/utils.ts` →
`lib/api/proto/poker.ts`, o módulo gerado pelo ts-proto inteiro: `ServerMessage` referencia
`TableSnapshot`, que referencia `Seat`/`Card`/`Pot`/`ChatMessage`/`TableReaction`/`LegalActions`/
`WinnerCardsRequest`/`RevealedSalt`. São ~157 kB de fonte gerada no caminho crítico de páginas que
nunca recebem um frame `state`. O próprio `MarketingQueryProvider` já documentava isso ("~1MB de
JS que uma página de texto não tem por que baixar") — e resolvia só para o grupo `(marketing)`.

E a cada `onOpen` do socket — inclusive o primeiro, o da carga da página — o bridge invalidava
`ROOM_BUCKETS_QUERY_KEY`, `['player','me']` e a raiz `['social']`. O `ws-client` reconecta em
backoff próprio, então acordar o notebook, trocar de rede ou um deploy do gateway produzia uma
rajada de opens e uma rajada de refetches.

## O que mudou

### 1. Dois codecs, um único wire

`proto/lobby.proto` é o subconjunto do gateway: **os mesmos números de campo** de
`poker.ServerMessage`, sem `TableSnapshot` e sem nada que só a mesa lê. Não é um segundo
protocolo — são exatamente os mesmos bytes; o proto3 descarta campos desconhecidos, então um frame
`state` que chegasse ao gateway degrada para os seus campos de lobby em vez de quebrar.

O campo 9 (`poker.Room`) vira uma mensagem vazia `RoomRef`: `room_created` só precisa saber que
veio uma sala junto — a lobby relê o agregado de buckets por HTTP (#205) — e manter o campo como
mensagem preserva a semântica de `undefined` quando ausente, igual ao codec completo.

| módulo | quem usa | proto |
|---|---|---|
| `lib/ws/codec.ts` | ambos | — (só o *framing*: inferência de `auth`, fatia de `ArrayBuffer`, ramo JSON de compatibilidade) |
| `lib/ws/utils.ts` | `useTableSocket` (rota `/table`) | `poker.proto` |
| `lib/ws/lobbyCodec.ts` | `useLobbyRealtime` (todas as rotas `(app)`) | `lobby.proto` |

`scripts/generate-proto.sh` gera os dois (o `lobby.proto` só para TypeScript — o servidor continua
codificando `poker.ServerMessage` completo), e `.github/workflows/proto.yml` verifica os dois
arquivos gerados.

**Regra de manutenção:** um campo visível para a lobby entra nos **dois** `.proto` com o **mesmo
número**; um campo que só a mesa lê entra só em `poker.proto`. `lobbyCodec.test.ts` decodifica
frames que ele mesmo produz com o codec completo, então um número de campo que divergisse falharia
no wire, não só nos tipos.

**Regressão travada por teste:** `lobbyCodec.test.ts` percorre o grafo de imports estáticos a
partir de `lib/providers/RealtimeBridge.tsx` (ignorando `import type`, que o compilador apaga) e
falha se `api/proto/poker.ts` reaparecer nele.

### 2. Reconnect coalescido

- **Trailing debounce** (`RECONNECT_RECONCILE_DEBOUNCE_MS`, 400 ms): cada open re-arma o timer;
  só o open em que a rajada termina gasta as leituras. Nada se perde por esperar — o último open
  sempre dispara.
- **O open da carga da página não é um reconnect** (`FIRST_OPEN_GRACE_MS`, 5 s a partir da
  montagem do hook): a mesma montagem que abriu o socket acabou de ler essas queries; não há
  janela offline para reconciliar, só uma leitura duplicada. O hook vive a sessão inteira, então
  uma indisponibilidade longa (liveness caída, token atrasado) abre pela primeira vez bem depois
  da montagem e **reconcilia** normalmente.
- **`refetchType: 'active'`** está escrito explicitamente (é o default do React Query) porque é o
  ponto: uma query não observada é marcada como stale, não relida. Um reconnect custa leituras
  para as superfícies que estão na tela, não para tudo que a sessão já carregou.

## O que não foi medido

O app não tem sink de métricas (`lib/telemetry.ts` é o sink de *erro*). "Refetches por reconnect" é
`lobbyReconcileCount()` — contador assertável nos testes e legível no console durante uma sessão
real, no mesmo formato de `settleRefetchReads()` / `sessionRefreshCount()` / `tickerIntervalCount()`.

O orçamento de JS inicial por rota autenticada já existe e continua sendo a régua:
`bundle-budget.json` + `npm run bundle:check` (job `quality`). Os números fixados **não foram
re-pinados** nesta mudança: eles só podem cair, e `check-bundle-budget.mjs` só falha em crescimento
— re-pinar exige `npm run build`, que deve ser feito deliberadamente no commit que quiser fixar o
novo teto.
