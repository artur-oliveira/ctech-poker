# Persistência de histórico de mãos + sessões + fairness verificável (B32)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fechar a lacuna documentada como "Phase 5 Task 12 não implementado" —
`poker_player_hands`/`poker_player_sessions` nunca são escritas em produção — e implementar B32 (`README.md:150-157`,
"commit-reveal is unverifiable by clients") publicando o hash de commit do shuffle no início da mão e revelando a
semente no fim, para que fairness seja auditável pelo cliente.

**Achado importante (verificado por leitura, não suposto):** boa parte da infraestrutura de histórico **já existe e está
em produção**, não é gap:

- `internal/tablestore` já grava toda ação em `poker_action_log` via `CommitAction`
  (`internal/tablestore/dynamo.go:112-171`), e `GET /tables/:tableId/hands/:handId/history`
  (`internal/api/v1/handhistory.go`) já lê isso — auditoria de ações por mão já funciona.
- `cmd/archiver` já arquiva `poker_action_log` em S3 antes do TTL expirar — retenção de longo prazo já existe.
- `internal/sessionlog.Store` (`RecordSession`/`ListSessions`/`RecordHand`/`ListHands`) e os endpoints
  `GET /players/me/sessions` / `GET /players/me/hands` (`internal/api/v1/playerhistory.go`) **já existem e já estão
  registrados** — o read-path está pronto.
- O gap real, confirmado por grep (`RecordSession`/`RecordHand` nunca aparecem fora de teste): o **write-path nunca é
  chamado** em código de produção. As tabelas ficam vazias para sempre porque ninguém escreve nelas.

**Architecture:** Três mudanças independentes, nesta ordem:

1. Propagar `handID` até o hook de fim-de-mão (`table.Actor.onHandComplete` hoje só recebe
   `hand.HandOutcome`, sem `handID` — `a.handID` é privado ao `Actor`). Mesmo padrão de mudança de assinatura já usado
   com sucesso nesta sessão para `tablemanager.NewManager`'s `roomLoader`
   (Feature A, Task 4) — trocar o tipo do parâmetro/campo, atualizar o único call site real, confirmar que os call sites
   de teste continuam compilando.
2. Estender `hand.HandOutcome` com `Payouts`/`Contributions map[string]int64` (ambos já computados dentro de
   `runShowdown`, só precisam ser anexados ao outcome antes de retornar) para que
   `sessionlog.HandItem.NetChange` seja calculável no hook sem reabrir o `Table`.
3. Chamar `sessionlog.RecordHand` no hook de fim-de-mão (`app.go`) e `sessionlog.RecordSession` nos pontos de
   entrada/saída do `buyin.Service` (`BuyIn`/`CashOut`), como dependências opcionais nil-checked — mesmo padrão já usado
   por `s.pending`/`s.players`/`s.rooms` no próprio `buyin.Service`.
4. B32: publicar `ShuffleCommitHash` (seguro publicar imediatamente, já documentado em
   `internal/engine/deck/deck.go:51`) no `Snapshot` assim que a mão começa, e revelar
   `ShuffleServerSeedHex` só quando `stage == Complete` (a mesma janela de tempo que `revealAll` já usa para hole
   cards — nunca revelar a semente enquanto a mão ainda pode ser influenciada por ela).

**Tech Stack:** Go (`internal/engine/hand`, `internal/table`, `internal/tablemanager`,
`internal/app`, `internal/buyin`, `internal/sessionlog`).

## Global Constraints

- `internal/engine/hand` continua sem import de `time`/rede — `ShuffleCommitHash`/`ServerSeed` já são campos de
  `deck.ShuffleResult`, nada novo a computar ali, só a exposição via `Snapshot`.
- `go test ./... -race`; testes com `//go:build integration` precisam do DynamoDB Local.
- Nenhuma chamada a `sessionlog`/hooks novos pode ser fatal ao fluxo principal — seguir exatamente o padrão já existente
  em `app.go`'s `onHandComplete` (`slog.Error` e seguir em frente, nunca abortar a mão por causa de uma falha de
  auditoria).

---

### Task 1: Propagar `handID` até o hook de fim-de-mão

**Files:**

- Modify: `../../api/internal/table/actor.go`
- Modify: `../../api/internal/tablemanager/manager.go`
- Modify: `../../api/internal/app/app.go`
- Test: `../../api/internal/table/actor_test.go` (integration)

**Interfaces:**

- `Actor.onHandComplete` muda de `func(hand.HandOutcome)` para `func(handID string, outcome
  hand.HandOutcome)`.
- `SetOnHandCompleteForActor(fn func(string, hand.HandOutcome))`.
- `notifyHandComplete` passa `a.handID` na chamada.
- `tablemanager.Manager.onHandComplete` muda de `func(tableID string, outcome hand.HandOutcome)` para
  `func(tableID, handID string, outcome hand.HandOutcome)`; mesma mudança em `NewManager`'s parâmetro
  `completion ...func(string, string, hand.HandOutcome)`.
- `app.go`'s `onHandComplete` closure ganha o parâmetro `handID string`.

- [ ] **Step 1: Escrever o teste que falha**

Em `actor_test.go`, estender (ou adicionar) um teste que verifica que o hook recebe um `handID`
não-vazio:

```go
func TestOnHandCompleteReceivesNonEmptyHandID(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)
	var gotHandID string
	a.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome) {
		gotHandID = handID
	})

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})
	stored, _ := store.LoadTable(context.Background(), "table-1")
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3})

	if gotHandID == "" {
		t.Fatal("expected onHandComplete to receive a non-empty handID")
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/table/... -run TestOnHandCompleteReceivesNonEmptyHandID -v
```

Esperado: FAIL (não compila — `SetOnHandCompleteForActor` ainda espera `func(hand.HandOutcome)`).

- [ ] **Step 3: Implementar**

Em `actor.go`:

```go
	onHandComplete               func(string, hand.HandOutcome)
```

```go
func (a *Actor) SetOnHandCompleteForActor(fn func(string, hand.HandOutcome)) { a.onHandComplete = fn }
```

Em `notifyHandComplete`:

```go
		if a.onHandComplete != nil {
			a.onHandComplete(a.handID, *outcome)
		}
```

Em `tablemanager/manager.go`:

```go
	onHandComplete func(tableID, handID string, outcome hand.HandOutcome)
```

```go
func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot), roomLoader func(string) (*roomstore.Room, bool, error), completion ...func(string, string, hand.HandOutcome)) *Manager {
	var onHandComplete func(string, string, hand.HandOutcome)
	if len(completion) > 0 {
		onHandComplete = completion[0]
	}
	...
}
```

No call site que registra o hook no ator recém-criado:

```go
	actor.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, handID, outcome)
		}
	})
```

Em `app.go`'s `newTableManager`:

```go
	onHandComplete := func(tableID, handID string, outcome hand.HandOutcome) {
		ctx := context.Background()
		unlocks, err := achv.RecordHand(ctx, tableID, outcome)
		// ... resto inalterado, handID disponível para a Task 3 usar
	}
```

- [ ] **Step 4: Rodar e confirmar passagem**

```bash
cd api && go test -tags integration ./internal/table/... -run TestOnHandCompleteReceivesNonEmptyHandID -v
cd api && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/actor_test.go api/internal/tablemanager/manager.go api/internal/app/app.go
git commit -m "feat(api): propagate handID through the hand-complete hook"
```

---

### Task 2: `Payouts`/`Contributions` em `HandOutcome`

**Files:**

- Modify: `../../api/internal/engine/hand/hand.go`
- Test: `../../api/internal/engine/hand/hand_test.go`

**Interfaces:**

- `HandOutcome` ganha `Payouts map[string]int64` e `Contributions map[string]int64` — ambos já computados como variáveis
  locais dentro de `runShowdown` (`payouts` e a fonte de `contributions`
  usada para `sidepots.ComputeSidePots`), só precisam ser anexados ao `outcome` antes de
  `t.lastOutcome = &outcome`.

- [ ] **Step 1: Escrever o teste que falha**

```go
func TestHandOutcomeIncludesPayoutsAndContributions(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	outcome := table.LastOutcomeForActor()
	if outcome.Payouts == nil || outcome.Contributions == nil {
		t.Fatal("expected HandOutcome to carry Payouts and Contributions")
	}
	if outcome.Contributions["P1"] == 0 && outcome.Contributions["P2"] == 0 {
		t.Fatal("expected non-zero contributions recorded for at least one player")
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/engine/hand/... -run TestHandOutcomeIncludesPayoutsAndContributions -v
```

- [ ] **Step 3: Implementar**

Em `hand.go`, `HandOutcome`:

```go
type HandOutcome struct {
	Winners            []string
	WinningCategory    string
	WonWithoutShowdown bool
	ComebackWinners    []string
	Participants       []string
	Payouts            map[string]int64
	Contributions      map[string]int64
}
```

Em `runShowdown`, a variável `contributions` já existe como `[]sidepots.Contribution` — construir um map paralelo (ou
reaproveitar, se já for indexável por ID) e anexar ambos ao `outcome`:

```go
	contributionsByID := make(map[string]int64, len(contributions))
	for _, c := range contributions {
		contributionsByID[c.PlayerID] = c.Amount
	}
	outcome := HandOutcome{
		Winners:            dedupeIDs(winningIDs),
		WonWithoutShowdown: wonWithoutShowdown,
		Participants:       participantIDs(t.handOrder),
		Payouts:            payouts,
		Contributions:      contributionsByID,
	}
```

(Inserir isso substituindo a construção atual de `outcome` — o resto do corpo, incluindo
`outcome.WinningCategory`/`outcome.ComebackWinners`, continua igual, só adicionando os dois campos novos.)

- [ ] **Step 4: Rodar e confirmar passagem**

```bash
cd api && go test ./internal/engine/hand/... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/hand_test.go
git commit -m "feat(api): carry per-player payouts and contributions on HandOutcome"
```

---

### Task 3: Escrever `poker_player_hands` e `poker_player_sessions`

**Files:**

- Modify: `../../api/internal/sessionlog/store.go`
- Modify: `../../api/internal/app/app.go`
- Modify: `../../api/internal/buyin/service.go`
- Test: `../../api/internal/sessionlog/store_test.go`, `../../api/internal/buyin/service_test.go`

**Interfaces:**

- New `func (s *Store) FindOpenSession(ctx context.Context, playerID, tableID string) (*SessionItem, error)`
  — varre as sessões mais recentes do jogador (`ListSessions`) e retorna a mais recente com
  `TableID == tableID && EndedAt == 0`.
- New `func (s *Store) CloseSession(ctx context.Context, item SessionItem) error` — apenas
  `RecordSession` de novo com a MESMA `PK`/`SK` (sobrescreve o item aberto com `EndedAt`/
  `CashoutAmount`/`NetPnL` preenchidos). Dynamo `PutItem` sobrescreve por chave — não precisa de update condicional aqui
  (não é caminho de correção monetária, é só auditoria; a fonte de verdade do saldo continua sendo `ctech-wallet`, isto
  aqui nunca credita/debita nada).
- `buyin.Service` ganha um campo opcional `sessions *sessionlog.Store` (nil-checked, mesmo padrão de
  `s.pending`) + um setter `SetSessionStore` (ou parâmetro de construtor — escolher o que já existir nos construtores
  `NewServiceWithGame`/`NewServiceWithPlayers`, adicionar sem quebrar call sites existentes: usar um setter é mais
  seguro contra quebrar assinatura).
- `app.go`'s `onHandComplete` chama `sessionStore.RecordHand` para cada `outcome.Participants[i]`.

- [ ] **Step 1: Escrever os testes que falham (sessionlog)**

```go
func TestFindOpenSessionReturnsTheMostRecentUnclosedSessionForTable(t *testing.T) {
	store := newTestStore(t) // reaproveitar o helper já usado em store_test.go
	ctx := context.Background()
	_ = store.RecordSession(ctx, SessionItem{PK: "p1", TableID: "t1", JoinedAt: 1})
	_ = store.RecordSession(ctx, SessionItem{PK: "p1", TableID: "t2", JoinedAt: 2})

	open, err := store.FindOpenSession(ctx, "p1", "t2")
	if err != nil {
		t.Fatalf("FindOpenSession: %v", err)
	}
	if open == nil || open.TableID != "t2" {
		t.Fatalf("expected the open session for t2, got %+v", open)
	}
}

func TestCloseSessionOverwritesTheSameItem(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.RecordSession(ctx, SessionItem{PK: "p1", SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 500})

	open, _ := store.FindOpenSession(ctx, "p1", "t1")
	open.EndedAt = 99
	open.CashoutAmount = 700
	open.NetPnL = 200
	if err := store.CloseSession(ctx, *open); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	sessions, _ := store.ListSessions(ctx, "p1", 10)
	if len(sessions) != 1 {
		t.Fatalf("expected the close to overwrite, not append — got %d items", len(sessions))
	}
	if sessions[0].EndedAt != 99 {
		t.Fatal("expected the overwritten item to carry EndedAt")
	}
}
```

(Checar `store_test.go` primeiro pra reaproveitar o helper de setup — se não existir um
`newTestStore(t)`, usar exatamente o padrão que os testes existentes de `RecordSession`/`ListSessions`
já usam.)

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/sessionlog/... -run "TestFindOpenSession|TestCloseSession" -v
```

- [ ] **Step 3: Implementar em `sessionlog/store.go`**

```go
func (s *Store) FindOpenSession(ctx context.Context, playerID, tableID string) (*SessionItem, error) {
	sessions, err := s.ListSessions(ctx, playerID, 50)
	if err != nil {
		return nil, err
	}
	for _, item := range sessions {
		if item.TableID == tableID && item.EndedAt == 0 {
			return &item, nil
		}
	}
	return nil, nil
}

func (s *Store) CloseSession(ctx context.Context, item SessionItem) error {
	return s.RecordSession(ctx, item)
}
```

- [ ] **Step 4: Wiring em `buyin.Service`**

Adicionar campo + setter em `service.go`:

```go
type Service struct {
	// ... campos existentes
	sessions *sessionlog.Store
}

func (s *Service) SetSessionStore(store *sessionlog.Store) { s.sessions = store }
```

Em `BuyIn`, depois que `actor.Dispatch(table.JoinCmd{...})` tiver sucesso (nenhum erro, incluindo o caso
`ErrAlreadySeated` tratado como no-op — **não** gravar sessão nesse caso, já existia sessão aberta):

```go
	if s.sessions != nil {
		if err := s.sessions.RecordSession(ctx, sessionlog.SessionItem{
			PK: playerID, TableID: roomID, BuyinAmount: amount, JoinedAt: dynamo.NowMillis(),
		}); err != nil {
			slog.Error("sessionlog: record session open failed", "player", playerID, "table", roomID, "err", err)
		}
	}
```

(Checar o nome exato do helper de timestamp em `gopkg.aoctech.app/api-commons/dynamo` — usar o mesmo que
`sessionlog.Store` já usa internamente, `time.Now().UnixMilli()` direto se não houver helper exportado.)

Em `CashOut`, depois de obter `stack` (chips finais) com sucesso:

```go
	if s.sessions != nil {
		if open, err := s.sessions.FindOpenSession(ctx, playerID, roomID); err == nil && open != nil {
			open.EndedAt = time.Now().UnixMilli()
			open.CashoutAmount = stack
			open.NetPnL = stack - open.BuyinAmount
			if err := s.sessions.CloseSession(ctx, *open); err != nil {
				slog.Error("sessionlog: close session failed", "player", playerID, "table", roomID, "err", err)
			}
		}
	}
```

- [ ] **Step 5: Wiring em `app.go`'s `onHandComplete`**

`newTableManager` precisa receber `sessionStore *sessionlog.Store` (novo parâmetro — atualizar o único call site real em
`registerRoutes`/onde quer que `newTableManager` seja chamado):

```go
	if sessionStore != nil {
		for _, id := range outcome.Participants {
			net := outcome.Payouts[id] - outcome.Contributions[id]
			result := "lost"
			for _, w := range outcome.Winners {
				if w == id {
					result = "won"
					break
				}
			}
			if err := sessionStore.RecordHand(ctx, sessionlog.HandItem{
				PK: id, TableID: tableID, HandID: handID, Outcome: result, NetChange: net, EndedAt: time.Now().UnixMilli(),
			}); err != nil {
				slog.Error("sessionlog: record hand failed", "table", tableID, "hand", handID, "player", id, "err", err)
			}
		}
	}
```

- [ ] **Step 6: Rodar tudo**

```bash
cd api && go test ./internal/sessionlog/... -race
cd api && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

- [ ] **Step 7: Commit**

```bash
git add api/internal/sessionlog/store.go api/internal/sessionlog/store_test.go \
        api/internal/app/app.go api/internal/buyin/service.go api/internal/buyin/service_test.go
git commit -m "feat(api): write poker_player_hands and poker_player_sessions (was read-only dead code)"
```

---

### Task 4: B32 — publicar commit hash, revelar semente no fim

**Files:**

- Modify: `../../api/internal/engine/hand/snapshot.go`
- Test: `../../api/internal/engine/hand/snapshot_test.go`

**Interfaces:**

- `Snapshot` ganha `ShuffleCommitHash string \`json:"shuffle_commit_hash,omitempty"\`` (hex de
  `t.shuffle.CommitHash`, sempre presente uma vez que a mão começou — "safe to publish immediately",
  comentário já existente em `deck.go:51`) e `ShuffleServerSeedHex string \`json:"shuffle_server_seed_hex,omitempty"\`
  ` (hex de `t.shuffle.ServerSeed`, só quando
  `t.stage == Complete` — nunca antes, ou um cliente poderia prever cartas futuras).

- [ ] **Step 1: Escrever os testes que falham**

```go
func TestViewForPublishesCommitHashAssoonAsHandStarts(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	view := table.ViewFor("p1")
	if view.ShuffleCommitHash == "" {
		t.Fatal("expected the shuffle commit hash to be published as soon as the hand starts")
	}
	if view.ShuffleServerSeedHex != "" {
		t.Fatal("must not reveal the server seed before the hand is complete")
	}
}

func TestViewForRevealsServerSeedOnlyOnceComplete(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	if view.ShuffleServerSeedHex == "" {
		t.Fatal("expected the server seed revealed once the hand is Complete")
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/engine/hand/... -run "TestViewForPublishesCommitHash|TestViewForRevealsServerSeed" -v
```

- [ ] **Step 3: Implementar**

Em `snapshot.go`, adicionar ao `Snapshot`:

```go
	ShuffleCommitHash    string `json:"shuffle_commit_hash,omitempty"`
	ShuffleServerSeedHex string `json:"shuffle_server_seed_hex,omitempty"`
```

Import `"encoding/hex"`. Em `ViewFor`, no `return Snapshot{...}`:

```go
	out := Snapshot{
		Stage:           stageNames[t.stage],
		Board:           boardCodes(t.board),
		Seats:           seats,
		Payouts:         t.payouts,
		Rake:            t.rakeCollected,
		CurrentPlayerID: current,
		LegalActions:    t.legalActionsFor(viewerID, current),
	}
	if t.shuffle != nil {
		out.ShuffleCommitHash = hex.EncodeToString(t.shuffle.CommitHash[:])
		if t.stage == Complete {
			out.ShuffleServerSeedHex = hex.EncodeToString(t.shuffle.ServerSeed[:])
		}
	}
	return out
```

(Checar o tipo exato de `ShuffleResult.ServerSeed` em `deck.go` — se já for `[]byte` em vez de
`[N]byte`, ajustar o slicing/`[:]` de acordo antes de compilar.)

- [ ] **Step 4: Rodar e confirmar passagem**

```bash
cd api && go test ./internal/engine/hand/... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/snapshot.go api/internal/engine/hand/snapshot_test.go
git commit -m "feat(api): publish shuffle commit hash immediately, reveal seed once the hand is complete (B32)"
```

## Self-Review Notes

- **A maior parte da infraestrutura já existia** (action log, archiver, sessionlog read-path, endpoints de leitura) —
  este plano NÃO recria nada disso, só fecha o write-path morto e adiciona B32, que era o gap real confirmado por grep,
  não suposição.
- **`handID` propagation (Task 1) segue exatamente o precedente já usado com sucesso** na Feature A (mudança de
  assinatura de `roomLoader` em `tablemanager.NewManager`) — mesmo tipo de mudança mecânica e de baixo risco.
- **Nenhuma escrita de auditoria é fatal ao fluxo principal** — todo erro vira `slog.Error`, nunca aborta a mão nem o
  buy-in/cash-out (mesmo padrão já usado por `achv.RecordHand`/
  `leaderboardSvc.RecordHand` em `app.go`).
- **B32 é uma decisão já catalogada** (README.md:150-157, `api/CLAUDE.md`'s "Other known issues") — este plano
  implementa o que já estava documentado como pendente, não inventa um requisito novo.
- **Fora de escopo:** nenhuma mudança em `cmd/handreplay` (é ferramenta de dev/replay manual, desconectada do dado real
  arquivado — não confundir com este plano). Features B e C já implementadas antes desta, conforme ordenação confirmada
  (bug3 → A → B → C → D).
