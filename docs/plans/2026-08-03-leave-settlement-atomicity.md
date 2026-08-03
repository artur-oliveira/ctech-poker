# Incidente + Correção — Leave não era atômico com o registro de settlement

Data: 2026-08-03 · Escopo: `api/internal/tablestore`, `api/internal/table`, `api/internal/buyin`

## 📌 Sintoma em produção

Durante uma janela em que a role da API ficou sem permissão de escrita em `poker_pending_cashouts`
(`pendingCashoutsTableArn`), dois jogadores reais (Dexther, Kelizinha) tiveram o assento removido da
mesa `01KZ42PEK6EN0GC288FVVGFSPV` sem receber crédito de volta na wallet sandbox. Uma nova tentativa de
`/leave` retornava `"hand: player %s not found"` (`hand.go:588`) — o assento já não existia mais.
Fichas recuperadas manualmente via crédito direto na wallet sandbox + fechamento manual do
`player_sessions` correspondente (628 e 23300, respectivamente). Ver conversa/relatório da investigação
para a reconstrução completa via CloudWatch + DynamoDB.

## 📌 Causa raiz (confirmada, não é só a falha de permissão)

`tablestore.CommitAction` (`dynamo.go:149`) recebe `extra ...types.TransactWriteItem` — é assim que
`buyin.Service` embute o registro de `poker_pending_cashouts` na MESMA transação que remove o jogador
da mesa. Antes da correção:

```go
items := []types.TransactWriteItem{stateTx, logTx}
if actionID != "" {
    ...
    items = append(items, extra...)          // só entra aqui dentro
    items = append(items, s.guards.BuildPutTxItemIfAbsent(guardItem))
}
```

`extra` só entrava na transação dentro do bloco `actionID != ""`. Só que `LeaveCmd`
(`internal/table/commands.go:145-151`) **nunca tem `ActionID`** — toda chamada de leave (manual, via
`buyin.Service.CashOut`, ou de sistema, via kick/AFK sweep em `actor.go`) sempre commita com
`actionID=""` (`actor.go:1578`). Ou seja: `extra` era descartado silenciosamente em **todo** leave,
não só durante a falha de permissão. A "atomicidade" que os comentários de `applyLeaveAndCommit`
(`actor.go:1563`) afirmam nunca existiu de fato — o que de fato grava o `poker_pending_cashouts` na
prática é a chamada separada e não-transacional `buyin.Service.settle()` (`service.go:481`), que roda
**depois** que a remoção do assento já commitou sozinha. Se essa chamada falhar por qualquer motivo
(a falha de permissão do incidente, um throttle, um timeout), o assento já se foi e não existe
nenhum registro de recuperação — o `reconcile` (Lambda, 5 min) nunca vai reconciliar algo que nunca
foi escrito.

## 📌 Correção

`items = append(items, extra...)` movido para fora do `if actionID != ""` — `extra` agora entra
sempre que não for vazio, independente de existir `actionID` (o guard de idempotência continua
condicional a `actionID`, que é a única coisa que realmente depende dele). Isso torna a remoção do
assento genuinamente atômica com o registro de settlement: se `poker_pending_cashouts` falhar, a
transação inteira falha, o `a.cached` é revertido (`applyLeaveAndCommit`, `actor.go:1579`) e o jogador
continua sentado — igual ao comportamento que o código já dizia (mas não fazia) ter.

Teste de regressão: `TestCommitActionIncludesExtraItemsWithoutActionID`
(`internal/tablestore/dynamo_test.go`) — commita com `actionID=""` e um `extra` item, confirma que o
item foi escrito. Verificado manualmente que falha sem a correção e passa com ela.

## 📌 Auditoria estrutural do restante da mesa (2026-08-03)

Mesmo padrão caçado no resto de `table/actor.go`, `tablemanager` e `buyin.Service.BuyIn`: handler
muta `a.cached` em memória, depois tenta commitar; caminho de erro que não reverte a mutação.

**Encontrado e corrigido:**

- **`applyJoinAndCommit` (`actor.go:1477`)** — mutava `a.cached` via `AddMidHandJoiner`/
  `AddWaitingPlayer` antes do commit, sem snapshot/rollback (ao contrário de
  `applyLeaveAndCommit`, que já tinha isso). Um commit falho por qualquer razão que não fosse
  version conflict deixava um jogador "fantasma" (com o stack do buy-in já debitado) confiável em
  memória — a próxima ação de QUALQUER outro jogador que commitasse com sucesso persistia esse
  fantasma pra valer, sem nunca ter existido um `poker_action_log` de "join". Corrigido com o
  mesmo padrão snapshot (`before := a.cached.ExportState()`) + rollback
  (`a.cached = hand.NewTableFromState(before)`) do Leave. Teste:
  `TestJoinRollsBackCacheOnNonConflictCommitFailure`
  (`internal/table/joinatomicity_integration_test.go`) — verificado que falha sem a correção.
- **`handleTurnTimeout`'s ramo de desconexão, `handleNextHand`, `handleRunoutStep`**
  (`actor.go:1404`, `:1723`, `:1783`) — todos só recarregavam o estado autoritativo em
  `ErrVersionConflict`; qualquer outro erro de commit (o mesmo `extra` descartado, um throttle)
  deixava a mutação fabricada em memória — no caso de `handleNextHand`, uma mão INTEIRA fabricada
  (cartas dealadas, blinds postados, fichas movidas pro pote) nunca persistida, esperando a
  próxima ação bem-sucedida de QUALQUER jogador pra virar real. Corrigido: recarrega
  incondicionalmente em qualquer erro (antes de decidir se propaga o erro — `ErrVersionConflict`
  continua sendo tratado como reconciliado/no-op, qualquer outro erro ainda propaga, mas agora com
  `a.cached` já saneado). Teste:
  `TestNextHandDiscardsFabricatedHandOnNonConflictCommitFailure`
  (`internal/table/nexthandatomicity_integration_test.go`) — verificado que falha sem a correção
  (`stage=PreFlop` fabricado sobrevivendo ao erro).

**Verificado e já seguro (nenhuma mudança):** `applyActAndCommit` (`actor.go:801`) tem o mesmo
formato mutate-then-commit, mas seu chamador `handleAct` (`actor.go:658-698`) já recarrega em
QUALQUER erro sempre que `a.trustCache` é true (o caso normal em produção — ator dono do lease), e
quando `trustCache` é false o próximo `ensureLoaded(ctx,false)` nunca é no-op, então também nunca
fica com cache poluído. Confirmado por leitura direta do código, não só do relatório da auditoria.

Suite completa (`go test ./...`, `go test -tags integration ./...`, `go test -race -tags
integration ./internal/table/... ./internal/tablestore/... ./internal/buyin/...`) passa sem
regressão após todas as correções.
