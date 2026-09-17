# Incidente — mesa travada no flop (runout all-in) + jogador que foldou recebendo pote

Data: 2026-09-17 · Escopo: `api/internal/table`, `api/internal/engine/hand`, `ctech-go-common/dynamo`

Mesa `01M2QYH151BYZJEYF3BAWZWYQK`, mão `01M2R0WT7EXVB8TB4GF8W9JWB5` (sandbox), 15:49–15:50 UTC.
Evidência: `quebrou_a_mesa.har`, `prod_poker_action_log`, `prod_poker_table_state_history`,
log group `/ctech-poker/prod/app`.

## 📌 Sintoma reportado

Luvas pausou (sit-out) durante a mão, Matheos deu all-in, Dexther (Thiago) foldou e Artur pagou.
Depois do call o jogo travou: nada avançava nem voltava. Só "destravou" quando Artur saiu da mesa
— e a mão foi para showdown com **três cartas no board**, sem turn e sem river.

## 📌 Timeline reconstruída (action log, versões 661–679)

| v | hora UTC | ação | resultado |
|---|----------|------|-----------|
| 661 | 15:48:49 | `next_hand` | SB Luvas (500), BB Matheos (1000), botão Artur |
| 666 | 15:49:00 | `not_ready` (Luvas) | Luvas **foldada na rodada de apostas**, `Player.State=Folded` |
| 667 | 15:49:04 | `request_exit` (Luvas) | `Player.State` vira **`SittingOut`** — perde o `Folded` |
| 669 | 15:49:08 | `all_in` (Matheos) | 10275 |
| 671 | 15:49:12 | `fold` (Dexther) | — |
| 672 | 15:49:14 | `call` (Artur) | rodada fecha, `advanceStage` entra no ramo de runout, **flop distribuído**, `RunoutPhase=1` |
| — | 15:49:17 | **runout_step que nunca existiu** | 47s de silêncio total nos dois processos |
| 673–677 | 15:49:40–58 | `request_exit`/`cancel_exit`/`ready` (Luvas) | commits passam, board continua com 3 cartas |
| 678 | 15:50:01 | `not_ready` (Artur) | Artur é **foldado à força**, sobra 1 jogador, `runShowdown` no board de 3 cartas |

Estado persistido conferido rodando o próprio engine sobre o item real de
`prod_poker_table_state_history`: no instante do travamento
`Stage=Flop`, `Round.IsComplete()=true`, `CurrentPlayerIDForActor()=""` e
**`IsAwaitingRunoutForActor()=true`**. Ou seja: o engine sabia que faltava turn/river; o que nunca
aconteceu foi o passo paced do ator.

## 📌 Causa raiz 1 — o timer de runout morre para sempre no primeiro commit que falha

`Actor.armRunoutTimer` (`api/internal/table/actor_timers.go`) é idempotente por
`(handID, stage, runoutPhase)` e **nunca limpa essa chave quando o timer dispara**. Já
`handleRunoutStep` engole `tablestore.ErrVersionConflict` (reload + `return nil`, sem log e sem
retry) e chama `broadcastAll()`, cujo `armRunoutTimer` cai exatamente na chave já registrada e
**não re-arma**. O mesmo vale para `rearmTimersFromCache`, que existe justamente para curar
timers perdidos: a chave em memória a bloqueia.

Resultado: uma única falha de commit no passo de runout congela a mão para sempre. Não há
`current_player_id`, então o turn timer também não resgata; só uma ação que mude
`(stage, phase)` sai desse estado.

Agravante — **classificação de erro compartilhada**: `Store.resolveCommitErr` decide "conflito de
versão" via `dynamo.IsConditionFailed`, que em `ctech-go-common` retorna `true` para *qualquer*
`TransactionCanceledException`, inclusive `TransactionConflict` (outra transação em voo no mesmo
item) e throttling. Como a instância roda **dois processos** (`app` e `app2`, confirmado nos
streams de log e já documentado em `2026-09-04-cross-instance-stale-turn-timer.md`), os dois
armaram o timer de runout a 109 ms de distância (15:49:14.097 e 15:49:14.206) e dispararam
juntos ~2,6s depois, escrevendo no mesmo item. Nenhum `runout_step` foi persistido (as versões do
action log são contíguas: 672 → 673 = `request_exit`) e **nada foi logado**, o que é compatível
apenas com o caminho silencioso de `ErrVersionConflict`.

## 📌 Causa raiz 2 — `SittingOut` sobrescreve `Folded` no meio da mão

`Table.RequestExit` (`hand.go`) termina com `if p.State != AllIn { p.State = SittingOut }`, sem
proteger quem já está `Folded`. Luvas foldou em v666 e virou `SittingOut` em v667. Como
`runShowdown` monta `sidepots.Contribution{Folded: p.State == Folded}`, ela entrou no showdown
como mão viva: pagamentos finais foram `Luvas 732` + `Matheos 19793`, isto é, ela **dividiu o
side pot de 1500 formado justamente pelos 500 que ela já havia foldado**. Isso contraria a regra
"dinheiro foldado é dinheiro morto". Também desalinha `countRemainingAndActable`/`activePlayers`
da `betting.Round` (lá ela é `Folded`, aqui é `SittingOut`).

## 📌 Causa raiz 3 — sit-out folda quem não está na vez

`Table.SitOutForActor` chama `t.round.Act(idx, Fold, 0)` para qualquer jogador `Active`, e
`betting.Round.Act` não checa ordem de ação. Dois efeitos reais nesta mão:

1. v666 setou `Round.LastActorID = Luvas`, e `actionScanOrder` ancora no próximo assento depois
   do último ator — o scan saltou de Dexther para Matheos (visível nos frames: `current_player_id`
   pula para o BB sem que Dexther e Artur tivessem agido).
2. v678 foldou **Artur depois de ele já ter pagado o all-in e fechado a ação**. Com isso
   `remaining == 1` e `runShowdown` rodou sem distribuir turn/river: Artur perdeu 10.275 em um
   pote contestado, com board de 3 cartas.

O próprio comentário de `RequestExit` já explica por que não se deve foldar quem não está no
relógio (é papel de `processPendingExitAutoFolds`); o caminho do toggle de ready
(`applyReadyAndCommit` → `SitOutForActor`) não respeita essa regra.

## 📌 Correções propostas

1. **`armRunoutTimer`**: a chave `(handID, stage, phase)` deve significar "há timer pendente", não
   "este ponto já foi armado uma vez" — limpar a chave quando o `AfterFunc` disparar, para que
   `broadcastAll`/`rearmTimersFromCache` possam re-armar.
2. **`handleRunoutStep`**: logar (WARN) e re-armar ao sair sem distribuir rua — inclusive no ramo
   de `ErrVersionConflict`. Nenhum caminho pode terminar com `IsAwaitingRunoutForActor()==true` e
   nenhum timer pendente.
3. **`RequestExit`/`SitOutForActor`**: nunca rebaixar `Folded` → `SittingOut` com mão viva; e só
   foldar via `round.Act` quando `t.currentPlayerToAct() == playerID`, deixando o resto para o
   sweep de pending-exit.
4. **Família (ctech-go-common)**: separar `TransactionConflict`/throttling de
   `ConditionalCheckFailed` em `dynamo.IsConditionFailed`, para que conflito real de versão não se
   confunda com erro retryável. Todos os serviços que usam `resolveTxErr`/`IsConditionFailed`
   (wallet, billing, account) têm a mesma exposição.
5. **Regressões**: teste de engine reproduzindo a sequência (sit-out do SB fora da vez → exit →
   all-in → fold → call) e teste de integração com dois atores concorrendo no mesmo item,
   garantindo que o runout se completa mesmo quando o primeiro `runout_step` falha.
