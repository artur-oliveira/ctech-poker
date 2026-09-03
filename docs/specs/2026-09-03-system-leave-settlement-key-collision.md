# Incidente + Correção — jogador não consegue sair de uma mesa da qual já saiu antes

Data: 2026-09-03 · Escopo: `api/internal/table`, `api/internal/tablemanager`, `api/internal/buyin`, `api/internal/app`

## 📌 Sintoma em produção

Vários HARs (`kely_nao_consegue_sair.har`, `sair_mesa_sozinho.har`, `quebrou_as.har`) com o mesmo
comportamento: o jogador clica em sair (`request_exit`), o servidor responde `action_ack`, o snapshot
volta com `pending_exit: true` — e nada mais acontece. O assento nunca é removido, a mesa trava, e
não há erro no console JS porque o servidor simplesmente engole a falha. Confirmado nos dois casos:

- **Kelizinha** (`953ac369…`), mesa `01M1KWY9ZVJ2THEF6ESXYQ71RX`: `request_exit` às 16:00, presa até
  as 16:04:55, quando um *idle sweep* (motivo `idle`, chave diferente) finalmente a removeu — depois
  do socket dela já ter caído.
- **Artur** (`78fd4d57…`), mesa `01M1KYZRT3QXYWZY724MBKM2VN`, sozinho na própria mesa: preso em
  `v14`/`waiting_for_players`/`pending_exit=true` (estado ainda travado no DynamoDB no momento da
  investigação).

Em ambos os casos existe uma linha **anterior e já resolvida** em `poker_pending_cashouts` com id
`<tableID>#<playerID>#system_leave#exit_requested` — de uma passagem anterior pela mesma mesa.

## 📌 Causa raiz

`buyin.Service.BuildSystemSettlementIntent` monta a linha imutável de recuperação
(`poker_pending_cashouts`) com a chave `fmt.Sprintf("%s#%s#system_leave#%s", roomID, playerID, reason)`
— **constante para um dado (mesa, jogador, motivo)**. Essa linha é escrita *create-only*
(`BuildPutTxItemIfAbsent`, `attribute_not_exists`) e **co-commitada na MESMA `TransactWriteItems` que
remove o assento** (correção de 2026-08-03, `docs/plans/2026-08-03-leave-settlement-atomicity.md` —
por design, se o registro de settlement falha, a remoção inteira falha e o jogador continua sentado).

Um jogador pode: sentar → ser removido pelo sistema → rebuy → ser removido de novo, na mesma mesa,
pelo mesmo motivo. Da segunda vez em diante, o `attribute_not_exists` da chave já-existente **cancela
a transação inteira** (`TransactionCanceledException[ConditionalCheckFailed]`). `applyLeaveAndCommit`
reverte `a.cached` (o assento volta), `handleLeave` só retenta em `ErrVersionConflict` (não é o caso),
e `removeEligiblePendingExits` faz `continue` **sem log**. O assento nunca sai. A linha antiga já
estar `resolved: true` não ajuda — a condição é sobre *existência*, não sobre estado.

`buyin.Service.CashOut` (leave iniciado pelo cliente) **já** resolvia isso: anexa um nonce do cliente
por clique (`"%s#%s#cashout#%s"`) exatamente com esse comentário. O caminho de remoção de sistema
(`BuildSystemSettlementIntent`) nunca recebeu o mesmo tratamento.

Por que o *idle sweep* eventualmente destrava: ele chama `handleLeave` com `reason: "idle"` →
chave `#system_leave#idle` → não colide com `#system_leave#exit_requested`. Por isso a Kelizinha saiu
4 minutos depois, como `idle`, e não como `exit_requested`.

## 📌 Correção

Um **nonce por remoção** (`Actor.newSettlementNonce`, ULID) gerado uma vez em cada ponto de remoção de
sistema (`removeEligiblePendingExits`, `removeIdlePlayersBetweenHands`, `handleAFKSweep`,
`handleKickTimeout`) e repassado **verbatim** por ambos os hooks:

- `Actor.systemSettlementIntent` (`+ settlementNonce string`) → `tablemanager.Manager` →
  `buyin.Service.BuildSystemSettlementIntent` (`+ nonce string`).
- `Actor.onPlayerRemoved` (`+ settlementNonce string`) → `tablemanager.Manager` → `app.go` →
  `buyin.Service.SettleSystemRemoval` (`+ nonce string`).

A chave passa a ser `<roomID>#<playerID>#system_leave#<reason>#<nonce>` (helper
`buyin.systemLeaveKey`, com fallback para a chave sem nonce quando `nonce == ""` — só para uma linha
pré-correção que ainda precise resolver). O nonce é o mesmo nas duas chamadas (intent co-commitado e
o `SettleSystemRemoval` que credita depois), senão o `reconcile` creditaria a carteira duas vezes sob
chaves diferentes.

Dedup entre instâncias **não** dependia dessa chave `IfAbsent` — depende do commit condicional por
`version` do próprio estado da mesa (ver comentário de `removeEligiblePendingExits`). Um nonce fresco
por tentativa apenas garante que o `IfAbsent` sempre passa; no máximo uma remoção de assento pode
commitar (condicional em `version`), então continua existindo no máximo uma linha de obrigação por
remoção real.

### Assinaturas alteradas (rodar `go vet -tags integration ./...` — feito)

`table.Actor.SetOnPlayerRemovedForActor`, `table.Actor.SetSystemSettlementIntentForActor`,
`tablemanager.Manager.SetOnPlayerRemoved`, `tablemanager.Manager.SetSystemSettlementIntent`,
`buyin.Service.BuildSystemSettlementIntent`, `buyin.Service.SettleSystemRemoval`.

## 📌 Testes

- `internal/buyin/key_test.go` — `TestSystemLeaveKeyIsUniquePerRemoval` (unit): nonces distintos →
  chaves distintas; nonce vazio → chave legada.
- `internal/table/pendingexit_settlement_collision_integration_test.go` —
  `TestRequestExitStillRemovesWhenAPriorSystemLeaveRowExists` (`//go:build integration`): pré-semeia a
  linha `#system_leave#exit_requested` já resolvida, dispara `request_exit`, verifica que o assento
  **é** removido e que uma nova linha com sufixo de nonce foi gravada. Falha sem a correção (assento
  preso).

## 📌 Limpeza de dados

Nenhuma migração necessária — linhas antigas `#system_leave#<reason>` continuam válidas e apenas
deixam de ser reusadas. Um assento atualmente preso em produção (ex.: Artur em
`01M1KYZRT3QXYWZY724MBKM2VN`) é destravado por: (a) deploy da correção + novo `request_exit`, (b) o
idle sweep após `kickGrace` de silêncio, ou (c) apagar manualmente a linha `resolved` colidente.
