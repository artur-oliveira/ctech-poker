# Frescor por família de query, não refetch global no foco (#233)

## O problema

O app inteiro compartilhava um único default — `staleTime: 30s` e
`refetchOnWindowFocus: true` — e exatamente **um** call site sobrescrevia isso (`/hands`). Voltar
para a aba depois de meio minuto relia catálogos, recibos de compra, históricos e sessões que nada
havia mudado, por cima do websocket que já empurra o que muda de fato.

## A classificação

`lib/queryFreshness.ts` tem a tabela inteira, e `createQueryClient` a aplica com
`setQueryDefaults` **por prefixo de chave** — assim classificar uma família não custa uma mudança
em cada um dos (muitos) call sites dela.

| preset | staleTime | foco | famílias |
| --- | --- | --- | --- |
| `SESSION_QUERY` | 30s | **refetch** | `['player','me']`, `['sessions']`, `['dailyReward']` |
| `STATIC_QUERY` | 30min | não | `['achievements','catalog']`, `['stakes']`, `['wallet','skus']`, `['wallet','reaction-catalog']`, `['wallet','cosmetic-catalog']` |
| `HISTORY_QUERY` | 5min | não | `['hands']`, `['wallet','sandbox-purchases']`, `['wallet','reaction-purchases']`, `['wallet','cosmetic-purchases']` |
| default | 30s | não | todo o resto |

O default do app passou a ser **sem refetch no foco**. Os dados vivos chegam por
`useLobbyRealtime`/`useTableRealtime`, que invalidam o que tocam a cada push **e** na abertura do
socket — ou seja, a reconexão já é o caminho de reconciliação. Refetch no foco virou exceção, e
toda exceção está nomeada acima.

Duas exceções fora da tabela, porque são locais por natureza:

- `usePurchaseStatus` liga `refetchOnWindowFocus: true` na própria query: o poll de fallback não
  roda com a aba escondida, então essa é a única leitura que recupera um frame perdido em segundo
  plano (o `staleTime` dela limita isso a **uma** leitura).
- `/hands` perdeu o override local: `['hands']` herda `HISTORY_QUERY`, com o mesmo efeito e o
  mesmo motivo (o refetch no foco relia *todas* as páginas carregadas — o relato "carregou mais
  sozinho").

Catálogos com 30 minutos de frescor são também o que faz Store e Table dividirem **uma** leitura do
mesmo catálogo de reações/cosméticos em vez de uma por rota. Posse continua mudando por
invalidação explícita (compra, estorno, websocket), nunca por relógio.

## Telemetria

O app não tem sink de métricas (`lib/telemetry.ts` é o sink de *erro* do cliente), então não há
contador de refetch por foco/reconnect/mutation. O substituto assertável é a própria tabela: como
o refetch no foco agora é opt-in e enumerável, `createQueryClient.test.ts` afirma qual família
recebe qual preset — inclusive que uma família não classificada não refaz fetch no foco.

## Como validar

`npx vitest run src/lib/providers/createQueryClient.test.ts src/lib/hooks/usePurchaseStatus.test.tsx "src/app/(app)/hands/page.test.tsx" --maxWorkers=2`.
