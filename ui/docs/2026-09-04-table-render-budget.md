# Teto de renders por snapshot na mesa (#230)

## O problema

`TableContent` renderiza a árvore inteira da mesa a cada mudança de estado do socket. E "mudança de
estado" não é só um frame `state`: um delta de `equity` troca o objeto do snapshot, uma mensagem de
chat troca `chat` + `chatBubbles`, uma reação troca `reactions`. Cada um desses re-renderizava
`TableStage`, os nove assentos (timers, count-ups, cartas), `Chat`, `TableReactions` e
`LastWinners`.

Colocar `memo` nesses componentes sozinho não resolveria nada: **nenhuma** prop que eles recebiam
era estável.

## As duas metades

Memoização só funciona se as duas metades existirem. As duas foram feitas juntas.

### 1. Identidades estáveis (a metade que faltava)

| onde | o que era | o que é |
|---|---|---|
| `useTableRealtimeSession` | 15 comandos como arrows inline no objeto de retorno — identidade nova a cada render | um único `useMemo` (`commands`), já que todos fecham só sobre refs, setState e o `emit`/`emitAux` que já eram `useCallback` |
| `useTableRealtimeSession` | `visibleChat`/`visibleBubbles`/`visibleReactions` recalculados por render quando há alguém mutado/bloqueado | `useMemo` — senão `chatBubbles` sozinho re-renderizaria o feltro a cada frame para quem já usou o mute |
| `useTableOverlays` | `panelOpenChange('chat')` criava um handler novo por chamada | cache por painel (`Map`), mesma identidade para sempre |
| `useTableOverlays` | `selectTableUtility`/`sendQuickReaction`/`sendTargetedReaction` como métodos de objeto literal | `useCallback` |
| `useSocialActions` | `{run, pending}` novo por render | `useMemo` — ele vai direto para o menu de cada assento |
| `TableStage` | `balancedSeatPosition(...)` devolve um objeto novo por assento por render | cache por `index:occupancy:orientation` (conjunto minúsculo e fechado) — **isto sozinho já derrubava `memo(Seat)`** |
| `TableStage` | `winnerStandings(snapshot)` por render | `useMemo` sobre o snapshot |
| `Seat` | `onEditNote`/`onReactionTarget`/`actionsMenu` fechavam sobre o assento no `TableStage` | recebem o assento como argumento (`onEditNote(seat)`, `onReactionTarget(playerId)`, `renderActionsMenu(seat)`), então nove assentos compartilham uma identidade |
| `page.tsx` | `playerNotesByID`, `relationshipsByID`, `favorites`, e os handlers inline do `TableStage`/`TableReactions` | `useMemo`/`useCallback`, declarados **acima dos early returns** (hooks não podem ser condicionais) |
| `page.tsx` | `seats={s.seats}` para `Chat` e `TableReactions` | `seatRoster`: uma projeção `SeatIdentity[]` reconstruída de uma chave, portanto estável enquanto o *roster* não muda (mesmo truque que o `suppressed` já usava). Pilha, pote e street deixaram de re-renderizar as duas asides |

### 2. As fronteiras de memo

`Seat`, `TableStage`, `Chat`, `TableReactions`, `LastWinners`. `ActionBar` fica de fora de
propósito: ela depende do snapshot em cada frame.

## Só props de temporização para quem está na vez

`nowMs`/`baseDeadlineMs`/`actionDeadlineMs`/`turnTimeoutMs` agora só vão para o assento da vez.
É idêntico em comportamento — `Seat` só consome `clockNow` sob `isTurn`
(`showNormalClock`/`showTimeBank` já exigiam `isTurn`, e `useLiveNow(Boolean(isTurn && ...))`
também) — e é o que impede `snapshotAt` de re-renderizar os outros oito a cada frame.

## O teto, medido

`src/components/table/tableRenderBudget.test.tsx`, perfil 9-max. `Seat` renderiza exatamente um
`PlayerAvatar` e nada mais no palco renderiza um, então mockar essa folha transforma renders de
assento em número assertável sem contador em código de produção.

| frame | renders de assento (de 9) |
|---|---|
| primeiro snapshot | 9 (um por assento) |
| balão de chat | **1** |
| delta de equity | **1** |
| frame `state` que move um jogador | **≤ 3** (o autor da ação + quem ganha/perde a vez) |

O delta de equity custa 1 porque `applySnapshotEquity` preserva a identidade dos assentos que não
mudaram — o teste depende disso de propósito, para que uma mudança no reducer apareça aqui.

## Frame time

Não medido, e não dá para medir aqui: jsdom não tem engine de layout, e o app não tem sink de
métricas (`lib/telemetry.ts` é o sink de *erro*). Renders por snapshot é o proxy assertável; o
frame time real precisa do profiler do Chrome numa mesa 9-max de verdade.

## O que não pode regredir

Memo não muda nada do que aparece, e nada aqui mexeu em `key`, em animação ou em ordem de DOM:

- Os anéis que capturam o offset no mount (`SeatTurnTimer`, `SeatTimeBank`, `HandOutcomeRing`)
  continuam com as mesmas `key`s e o mesmo momento de montagem.
- `renderActionsMenu(seat)` produz o mesmo elemento que `actionsMenu` produzia; só passou a ser
  construído dentro do `Seat` em vez de fora dele.
- O cache de `balancedSeatPosition` devolve os mesmos números — `--seat-s`/`--seat-t` não mudam.
- Landmarks, `aria-live`, rótulos e ordem de foco: intocados. Os testes de axe das rotas continuam
  verdes.
