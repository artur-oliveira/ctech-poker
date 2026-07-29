# Auditoria técnica — ctech-poker `api/` + integrações frontend (2026-07-27)

## Contexto

Pedido: auditoria sem suavização, cobrindo performance, gerenciamento de estado, bugs reais e
funcionalidades novas. Ponto de partida: a arquitetura (DynamoDB conditional writes + 1 goroutine
`Run` por mesa) de fato é sólida e isso foi confirmado por leitura direta de código, não é elogio
fácil. Mas há bug de dinheiro real e duas falhas de disponibilidade que merecem atenção antes de
qualquer coisa nova.

Metodologia: 3 varreduras de código paralelas (engine/actor, tablemanager/buyin/tablestore, camada
API+WS+frontend), cruzadas contra as alegações de `agy_audit.md`, o plano de remediação
`docs/plans/2026-07-19-api-audit-remediation.md` (T1-T12) e as afirmações do `CLAUDE.md`. Os 3
achados mais graves abaixo foram reverificados por leitura direta do código (não só o relato dos
agentes).

---

## O que está realmente resolvido (não retrabalhar)

Confirmado por leitura direta:

- **T1-T11** (liveness de actor morto, fail-fast sem Redis em prod, idempotência estável de
  buy-in/cash-out, auto-fold só dentro de `Run`, retry de conflito em SitOut, re-arm de blind
  escalation, lock em `GetOrCreateActor`, seed condicional único, equity fora do hot path,
  reconciliação de cash-out) — todos **fixed**, com evidência de file:line.
- **B9** (JWT `sub`+`sid` obrigatórios, M2M nunca age como jogador) e **B32** (commit-reveal do
  shuffle) — confirmados.
- As 3 alegações do `agy_audit.md` de terceiros — **nenhuma reproduz**: o cap de all-in em
  `betting.go` já trata explicitamente o exploit descrito (tem até comentário citando o cenário);
  `disconnectedSince` já é mapeado pro snapshot ao vivo via `applyPresence`; `isTurn` no frontend já
  usa igualdade estrita de `current_player_id`. Esse "audit_report.md" de terceiro parece ter
  auditado uma versão anterior do código ou nunca bateu com o que foi de fato commitado.
- Nenhum IDOR encontrado — todo handler mutante deriva o ID do JWT, nunca de body/path.
- `T10`: rate limit Redis em `POST /rooms`/`join`/`sandbox-credits`, `sanitizeRoom` no share-code,
  paginação por cursor real (não só documentada).

---

## Bugs reais (ranqueados)

### 1. CRÍTICO — corrida de buy-in perde dinheiro real silenciosamente

`internal/buyin/service.go:160-211`

`isSeated` (linha 160) é um TOCTOU: duas requisições quase simultâneas de `BuyIn` para o mesmo
(room, player) — duplo clique, dois dispositivos — passam pela checagem `seated && stack > 0` antes
de qualquer uma ter commitado. Cada uma gera uma **nonce distinta** (client-supplied, "estável por
clique" segundo o comentário da própria idempotency key), então o wallet **não dedupe** as duas — as
duas debitam de verdade. O `Actor` serializa os dois `JoinCmd`s; o segundo bate no guard
`Stack>0` de `ErrAlreadySeated` (`hand.go:410-413`) e o handler (linha 198-199) faz:

```go
if errors.Is(joinErr, hand.ErrAlreadySeated) {
    return nil   // sucesso — SEM reembolso
}
```

O dinheiro do segundo débito nunca volta, nunca vira pending record, não gera erro pro cliente.
Isso é dinheiro real desaparecendo sem rastro, no exato caminho que Fase 5 (dinheiro real) usa.
**Prioridade máxima antes de dinheiro real escalar em produção.**

Fix: mover a checagem de "já sentado" para dentro do mesmo `commit()` condicional (não fora, antes
do débito), ou tratar `ErrAlreadySeated` retornando refund igual ao branch de erro logo abaixo
(linhas 201-210) em vez de `return nil`.

### 2. ALTO — uma conexão travada trava a mesa inteira (DoS trivial)

`internal/api/v1/tablews.go:650-654`

```go
func (w *wsConnAdapter) WriteMessage(messageType int, data []byte) error {
    return w.conn.WriteMessage(fws.BinaryMessage, data)
}
```

`wsWriteWait` (linha 37) só é usado no `WriteControl` de ping (linha 632) — **nunca** em
`WriteMessage`. `broadcastAll` (`actor.go:1406-1455`) escreve pra cada jogador sincronamente, dentro
da única goroutine `Run` da mesa. Um cliente que abre o WS e simplesmente para de ler (enche o
buffer TCP — não precisa bypass de auth, 1 conexão basta) faz essa escrita bloquear por minutos no
nível do SO, travando `act`/`fold`/`raise` de **todos os outros jogadores** daquela mesa até o
kernel resetar o socket.

Fix: `conn.SetWriteDeadline(time.Now().Add(wsWriteWait))` antes de todo `WriteMessage`, tratar
timeout como desconexão do destinatário (nunca bloquear os outros).

### 3. ALTO — Actor de instância sem lease nunca é liberado (vazamento de memória/goroutine)

`internal/tablemanager/manager.go:166-168`, `manager.go:220-232`

Quando a instância **não** ganha a lease (`trustCache=false`), roda `go actor.Run(context.Background())`
sem nenhum gatilho de cancelamento — `Done()`/`IsAlive()` desse actor nunca fecham. `Release()` só é
chamado em `DrainAndRelease` (shutdown gracioso). Como qualquer instância da fleet pode servir
tráfego WS de qualquer mesa (arquitetura distribuída, por design), toda instância que já tocou uma
mesa que não é sua acumula um `Actor` (com seus maps e timers) e uma goroutine bloqueada, pra sempre,
enquanto o processo viver. Sob rotatividade real de mesas isso cresce sem limite.

Fix: eviction por ociosidade — se um actor sem lease ficar N minutos com 0 conexões WS ativas,
cancelar o `runCtx` e chamar `removeActor`, mesmo sem perda de lease.

### 4. MÉDIO — analytics de fim de mão bloqueiam o hot path (mesma classe de bug que T9 corrigiu, mas não pra tudo)

`internal/table/actor.go:1454` → `app.go:250-279`

`onHandComplete` roda **síncrono** dentro de `Run`: achievements, `RecordUnlocks`, `RecordHand`
(loop CAS de até 5 round-trips), `LoadActionsSince` + `pokerStatsStore`, e um loop de
`sessionStore.RecordHand` por participante. Isso bloqueia o próximo comando daquela mesa exatamente
como a equity fazia antes do T9 — só que ninguém corrigiu esse caminho.

### 5. MÉDIO — `IncrementAchievementPoints` faz N escritas em vez de 1

`internal/leaderboard/store.go` — `for i := 0; i < points; i++ { AtomicIncrement(...) }`. Um unlock
de 50 pontos = 50 `UpdateItem` sequenciais em vez de um único `ADD :points`.

### 6. MÉDIO — scan de reconciliação sem paginação e sem TTL

`cmd/reconcile/pending.go:81-108` faz `db.Scan` cru, sem loop de `LastEvaluatedKey`.
`PendingCashout` não tem campo TTL (diferente de toda outra tabela do repo) — entradas resolvidas
nunca são apagadas, o scan só cresce e pode truncar silenciosamente ao passar de 1MB por página.

### 7. BAIXO — vazamento de mensagem de erro interna pro cliente

`rooms.go:276` e `leaderboard.go:21` repassam `err.Error()` cru de `buyin`/wallet-client direto na
resposta HTTP. Baixo risco hoje, mas qualquer erro futuro de DB/wallet embrulhado vaza texto verbatim.

### 8. BAIXO — timers de som de reveal não cancelados

`ui/src/lib/hooks/useTableRealtime.ts:126-129` — `setTimeout` de `playSound('reveal')` por carta
extra, sem ref, sem cleanup no unmount nem quando chega um novo snapshot no meio da animação.

---

## Performance

- **Equity Monte Carlo é caro e sem controle**: `actor.go:1448` cria 1 goroutine nova por
  jogador-com-oponentes a cada `broadcastAll` (ou seja, a cada ação), sem debounce contra um cálculo
  já em andamento pro mesmo assento/versão. Cada estimativa faz 500 iterações e cada iteração chama
  `crypto/rand.Read` **por carta sorteada** — numa mesa de 8, isso é ~8.000 syscalls de RNG
  criptográfico por cálculo, multiplicado por até 9 goroutines concorrentes por ação. `Best7` avalia
  as 21 combinações C(7,5) sem tabela de lookup, chamado ~500×(1+oponentes) vezes por estimativa.
  Isso é custo de CPU e syscall real e recorrente no caminho mais quente do sistema — corte de
  iterações (500→150-200 já é suficiente pra UI), pool de `math/rand` semeado (não precisa ser
  criptográfico pra uma estimativa de equity), debounce por versão de snapshot.
- **N+1 rebuild de snapshot por ação**: `commit()` monta um `ViewFor("")` completo só pro `Frame` de
  auditoria, e `broadcastAll` monta mais N (`ViewFor(viewerID)` por jogador sentado) pro mesmo
  estado. Baixo por estar limitado a ~9 assentos, mas é trabalho duplicado em toda escrita.
- **Leitura obrigatória no DynamoDB a cada `SnapshotCmd` explícito** (`actor.go:258`) — intencional
  por correção multi-instância, mas qualquer cliente que faça polling em vez de confiar no push gera
  1 read por poll.
- Itens #4 e #5 da lista de bugs acima também são achados de performance, não só correção.

## Gerenciamento de estado

- A arquitetura central (correção via DynamoDB conditional write + único goroutine `Run` por mesa +
  lease só pra latência) **é o ponto forte real do sistema** — nenhuma race foi encontrada nela, os
  timers (`time.AfterFunc`) sempre despacham comando em vez de mutar estado direto, e a goroutine
  solta de equity nunca toca `a.cached` (trabalha sobre cópia defensiva). Isso não é elogio fácil:
  foi verificado por leitura direta.
- O ponto fraco real é o **ciclo de vida do actor** (achado #3 acima) — o modelo de correção de
  estado da mesa está certo, mas o modelo de liberação de recursos por instância não acompanha.
- **Gap de autorização por usuário**: `authMiddleware` só olha `sub`/`sid`, nunca `claims.Scope` nem
  `claims.KYCLevel` (ambos existem no tipo `jwtverify.Claims`). O gate de dinheiro real é 1 booleano
  global (`cfg.RealMoneyEnabled`), não por usuário/KYC — qualquer jogador autenticado pode entrar
  numa mesa de dinheiro real assim que a flag estiver ligada, com ou sem KYC aprovado. Isso soma aos
  2 gaps já conhecidos no `ctech-account` (scope `game-status` e `debit-real` ausentes do catálogo).

---

## Funcionalidades novas

`future.md`/`future_analysis.md` já cobrem isso de forma exaustiva (300+ linhas, com custo e
priorização) — sem repetir aqui. Duas críticas reais a esses documentos, e 3 ideias que não estão
neles:

- **Os documentos de roadmap já estão desatualizados em relação ao código**: `action pre-selectors`,
  `rabbit hunting`, `reality check` e `time bank` — todos listados como "F0, Implementar agora" em
  `future_analysis.md` (26/07) — **já existem implementados** (`ui/src/lib/actionPreselection.ts`,
  `ui/src/components/table/RabbitHunt.tsx`, `RealityCheck.tsx`, `internal/table/timebank_test.go` +
  `hand/timebank_test.go`). Ou a análise de custo/prioridade foi feita sem checar o código atual, ou
  o código andou mais rápido que o documento — de qualquer forma, ressincronize esse doc antes de
  usá-lo pra priorizar o próximo sprint, porque hoje ele está recomendando trabalho já feito.
- **3 ideias novas, amarradas nos bugs reais encontrados aqui** (não no brainstorm genérico do
  `future.md`):
    1. Toggle opt-in de equity por jogador — já que o cálculo é caro (ver Performance acima) e nem
       todo jogador olha esse número, desligar por padrão pra quem não usa corta custo real de CPU.
    2. Métrica de backpressure por mesa (tempo de escrita WS, conexões lentas) — pra pegar o achado #2
       em produção antes que vire um DoS explorado, não só depois de corrigido.
    3. Auditoria/dedupe de idempotência no `walletclient` — o padrão de corrida do achado #1
       provavelmente se repete em qualquer outro caminho que gere nonce no cliente; vale uma camada
       central de dedupe em vez de confiar em cada call site fazer certo.

---

## Ordem recomendada, se for corrigir

1. **#1** (corrida de buy-in) — bug de dinheiro real, ativo hoje.
2. **#2** (WS sem deadline) — fix trivial (`SetWriteDeadline`), ganho grande de disponibilidade.
3. **#3** (vazamento de actor) — eviction por ociosidade.
4. Achados médios (#4-#6), na ordem que preferir.

## Verificação

- `go test ./... -race` continua sendo o gate — os 3 fixes prioritários pedem teste novo cada:
  corrida de buy-in (duas `BuyIn` concorrentes com nonces distintas, assert reembolso ou dedupe),
  `SetWriteDeadline` (mock conn que nunca lê, assert que outras conexões da mesma mesa continuam
  recebendo broadcast), eviction de actor (actor sem lease, 0 conexões por N minutos, assert
  `IsAlive()==false` e remoção do map).
- Não há UI nova aqui além do que já existe — não precisa rodar frontend pra validar esses fixes,
  exceto o achado #8 (timers de som), que pede teste manual no navegador.
