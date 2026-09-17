# Table Snapshot Divergence & Highlight Winner — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer o "Maior pote de hoje" nomear o vencedor real mesmo sem showdown, e garantir que dois frames com o mesmo `(hand_id, snapshot_version)` sejam idênticos qualquer que seja a instância que os emitiu.

**Architecture:** Quatro mudanças independentes. (1) `highlights.Highlight` passa a persistir o vencedor vindo de `hand.HandOutcome`, e a UI usa esse campo com fallback para o caminho atual. (2) `handleExternalChange` deixa de republicar — a entrega via `ws.RedisRegistry` já é fleet-wide, então o publish da instância que commitou basta; a irmã continua recarregando estado, re-armando timers e rodando as varreduras. (3) O Monte-Carlo de equity passa a usar semente derivada das entradas, então toda instância produz o mesmo número. (4) O streak publicado no fim da mão dispara um `ChangeNotifier`, e a irmã força uma releitura do badge quando o estágio recarregado é `complete` — fechando a janela de 30 s sem voltar a ler Valkey por comando.

**Tech Stack:** Go 1.25 (`api/`), Next.js 16 + React 19 + Vitest (`ui/`), protobuf sobre WebSocket, DynamoDB, Valkey.

**Spec:** `docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md`

## Global Constraints

- Testes Go: `cd api && go test ./... -race`. Testes UI: `cd ui && npm test`.
- Nada de campo novo no proto do snapshot nesta entrega — R3/R4 são resolvidos no servidor.
- Nenhuma leitura Valkey adicional **por comando** (R6). Por mão é aceitável.
- Rows de highlight escritas antes do deploy não têm o campo novo e precisam continuar renderizando (R2).
- Comentários no código em inglês, seguindo o estilo denso-explicativo já usado nos arquivos tocados.
- Mensagens de commit sem trailers de atribuição.

---

## File Structure

| Arquivo | Responsabilidade | Ação |
|---|---|---|
| `api/internal/highlights/store.go` | `Highlight` + `RecordHand` + `winnersOf` | Modificar |
| `api/internal/highlights/store_test.go` | Unidade de `winnersOf` | Modificar |
| `ui/src/lib/api/highlights.ts` | Tipo `TableHighlight` | Modificar |
| `ui/src/components/table/TodayHighlight.tsx` | Caption do card | Modificar |
| `ui/src/components/table/TodayHighlight.test.tsx` | Testes do caption | Modificar |
| `api/internal/table/actor_views.go` | `sync`/`publishSnapshots`/`applyStreaks` | Modificar |
| `api/internal/table/actor_hooks.go` | `notifyHandComplete` devolve se rodou | Modificar |
| `api/internal/table/actor_presence.go` | `handleExternalChange` | Modificar |
| `api/internal/table/changenotify_test.go` | Contrato do external change | Modificar |
| `api/internal/table/streakfreshness_test.go` | Frescor do badge no fim da mão | Criar |
| `api/internal/engine/equity/equity.go` | Semente determinística | Modificar |
| `api/internal/engine/equity/equity_test.go` | Determinismo | Modificar |

---

### Task 1: Highlight persiste o vencedor

**Files:**
- Modify: `api/internal/highlights/store.go`
- Test: `api/internal/highlights/store_test.go`

**Interfaces:**
- Consumes: `hand.HandOutcome{Winners []string, Payouts map[string]int64, PotResults []PotResult}`, `names map[string]string`.
- Produces: `highlights.HighlightWinner{PlayerID string, Name string, Payout int64}` e o campo `Highlight.Winners []HighlightWinner` (JSON `winners`, dynamodbav `winners`). Task 3 consome o JSON.

- [x] **Step 1: Escrever o teste que falha**

Acrescentar ao fim de `api/internal/highlights/store_test.go`:

```go
// The reported bug: an all-in everybody folds to has no showdown, so
// revealedHandsOf returns nothing and the card rendered with no name at all
// — even though the outcome names the winner outright.
func TestWinnersOf_NamesTheWinnerWithoutAShowdown(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:            []string{"p1"},
		WonWithoutShowdown: true,
		Participants:       []string{"p1", "p2"},
		Payouts:            map[string]int64{"p1": 150875},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Qd"}, Revealed: false},
			"p2": {HoleCards: [2]string{"2c", "7s"}, Revealed: false},
		},
	}
	names := map[string]string{"p1": "Artur 1234", "p2": "Dexther"}

	got := winnersOf(outcome, names)

	if len(got) != 1 {
		t.Fatalf("expected 1 winner, got %d: %+v", len(got), got)
	}
	if got[0].PlayerID != "p1" || got[0].Name != "Artur 1234" || got[0].Payout != 150875 {
		t.Fatalf("unexpected winner: %+v", got[0])
	}
	if revealed := revealedHandsOf(outcome, names); len(revealed) != 0 {
		t.Fatalf("no showdown happened, so nothing may be revealed: %+v", revealed)
	}
}

// A split pot names everyone who was paid, ordered by payout then id so the
// caption is stable between reads.
func TestWinnersOf_SplitPotIsOrderedAndComplete(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:      []string{"p2", "p1"},
		Participants: []string{"p1", "p2"},
		Payouts:      map[string]int64{"p1": 500, "p2": 1500},
	}

	got := winnersOf(outcome, map[string]string{"p1": "Alice", "p2": "Bob"})

	if len(got) != 2 {
		t.Fatalf("expected 2 winners, got %+v", got)
	}
	if got[0].PlayerID != "p2" || got[0].Payout != 1500 {
		t.Fatalf("biggest payout must come first, got %+v", got)
	}
	if got[1].PlayerID != "p1" || got[1].Payout != 500 {
		t.Fatalf("unexpected runner-up: %+v", got[1])
	}
}

// A winner with no payout entry is still named — the pot layer bookkeeping is
// PotResults' job, and dropping the name would reproduce the original bug.
func TestWinnersOf_WinnerWithoutAPayoutEntryIsStillNamed(t *testing.T) {
	outcome := hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1"}}

	got := winnersOf(outcome, nil)

	if len(got) != 1 || got[0].PlayerID != "p1" || got[0].Payout != 0 {
		t.Fatalf("unexpected winners: %+v", got)
	}
}
```

- [x] **Step 2: Rodar e confirmar que falha**

Run: `cd api && go test ./internal/highlights/ -run TestWinnersOf -race`
Expected: FAIL — `undefined: winnersOf`.

- [x] **Step 3: Implementar**

Em `api/internal/highlights/store.go`, logo depois do bloco `RevealedHand`, adicionar:

```go
// HighlightWinner is who actually won the pot, copied straight from
// hand.HandOutcome. It exists because Revealed above is empty whenever the
// hand ended without a showdown (an all-in everybody folds to), which left
// the card with a pot and no name — the winner was known server-side and
// thrown away. It also fixes the attribution the UI could only guess at:
// with a side pot, the best hand shown is not necessarily who won the most.
type HighlightWinner struct {
	PlayerID string `dynamodbav:"player_id" json:"player_id"`
	Name     string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	Payout   int64  `dynamodbav:"payout" json:"payout"`
}
```

No struct `Highlight`, adicionar o campo entre `Board` e `Revealed`:

```go
	Winners    []HighlightWinner `dynamodbav:"winners,omitempty" json:"winners,omitempty"`
```

Em `RecordHand`, preencher o campo no literal `Highlight`:

```go
	item := Highlight{
		TableID: tableID, Date: time.Now().UTC().Format("2006-01-02"),
		HandID: handID, Pot: pot, Board: outcome.Board,
		Winners:  winnersOf(outcome, names),
		Revealed: revealedHandsOf(outcome, names), RecordedAt: time.Now().UnixMilli(),
	}
```

Ao fim do arquivo, junto de `revealedHandsOf`:

```go
// winnersOf copies the hand's actual winners — unlike revealedHandsOf, which
// is gated on a showdown having happened. Ordered by payout descending, then
// by player id, so a split pot renders the same caption on every read.
func winnersOf(outcome hand.HandOutcome, names map[string]string) []HighlightWinner {
	winners := make([]HighlightWinner, 0, len(outcome.Winners))
	seen := make(map[string]struct{}, len(outcome.Winners))
	for _, id := range outcome.Winners {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		winners = append(winners, HighlightWinner{PlayerID: id, Name: names[id], Payout: outcome.Payouts[id]})
	}
	if len(winners) == 0 {
		return nil
	}
	sort.Slice(winners, func(i, j int) bool {
		if winners[i].Payout != winners[j].Payout {
			return winners[i].Payout > winners[j].Payout
		}
		return winners[i].PlayerID < winners[j].PlayerID
	})
	return winners
}
```

Adicionar `"sort"` ao bloco de imports.

- [x] **Step 4: Rodar e confirmar que passa**

Run: `cd api && go test ./internal/highlights/ -race`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add api/internal/highlights/store.go api/internal/highlights/store_test.go
git commit -m "fix(highlights): persist the hand's real winner, not only revealed hands"
```

---

### Task 2: UI nomeia o vencedor persistido

**Files:**
- Modify: `ui/src/lib/api/highlights.ts`
- Modify: `ui/src/components/table/TodayHighlight.tsx`
- Test: `ui/src/components/table/TodayHighlight.test.tsx`

**Interfaces:**
- Consumes: `TableHighlight.winners?: HighlightWinner[]` da Task 1.
- Produces: `highlightWinnerLabel(board?, revealed?, winners?)` — assinatura estendida, terceiro parâmetro opcional; chamadores existentes continuam válidos.

- [x] **Step 1: Escrever os testes que falham**

Acrescentar dentro do `describe('TodayHighlight', …)` em `ui/src/components/table/TodayHighlight.test.tsx`:

```tsx
  test('names the winner of a hand that had no showdown', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      pot: 150875,
      board: ['As', '3s', '2s', '8h', 'Ac'],
      winners: [{player_id: 'p1', name: 'Artur 1234', payout: 150875}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Artur 1234')).toBeInTheDocument());
  });

  test('appends the made hand when the winner also showed their cards', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ac', '7d', '2s', '9h', '3c'],
      winners: [{player_id: 'p1', name: 'Alice', payout: 1500}],
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice — Par')).toBeInTheDocument());
  });

  test('names the paid winner, not the best hand shown, on a side pot', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ac', '7d', '2s', '9h', '3c'],
      winners: [{player_id: 'p2', name: 'Bob', payout: 9000}],
      revealed: [
        {player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Ad']},
        {player_id: 'p2', name: 'Bob', hole_cards: ['9c', '9s']},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText(/^Bob/)).toBeInTheDocument());
    expect(screen.queryByText(/Alice/)).not.toBeInTheDocument();
  });

  test('joins every winner of a split pot', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      winners: [
        {player_id: 'p1', name: 'Alice', payout: 750},
        {player_id: 'p2', name: 'Bob', payout: 750},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice e Bob')).toBeInTheDocument());
  });

  test('still reads the revealed hands on a row written before winners existed', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice — Par')).toBeInTheDocument());
  });
```

- [x] **Step 2: Rodar e confirmar que falha**

Run: `cd ui && npx vitest run src/components/table/TodayHighlight.test.tsx`
Expected: FAIL — os quatro primeiros testes não acham o texto (o caption ignora `winners`).

- [x] **Step 3: Implementar o tipo**

Em `ui/src/lib/api/highlights.ts`, acima de `TableHighlight`:

```ts
export interface HighlightWinner {
  player_id: string;
  name?: string;
  payout: number;
}
```

E dentro de `TableHighlight`, entre `board` e `revealed`:

```ts
  winners?: HighlightWinner[];
```

- [x] **Step 4: Implementar o caption**

Em `ui/src/components/table/TodayHighlight.tsx`, substituir o bloco de comentário
`// KNOWN LIMITATION: …` e a função `highlightWinnerLabel` inteira por:

```tsx
// Names whoever the server says was PAID (`winners`, from hand.HandOutcome),
// which is the only source that survives a hand nobody showed down — the
// reported bug, where an all-in everybody folded to left this card with a pot
// and no name. It also settles the attribution the old caption could only
// guess at: with a side pot the best hand among `revealed` is not necessarily
// who won the most, and `winners` carries the per-player payout to sort by.
// `revealed` is now only consulted to decorate the caption with the made hand,
// and as the whole answer for rows written before `winners` existed.
function madeHandOf(board: string[] | undefined, hole: string[]) {
  if (board?.length !== 5 || new Set(board.map(card => card.toUpperCase())).size !== 5 ||
    board.some(card => !CARD_CODE.test(card))) return undefined;
  if (hole.length !== 2 || !hole.every(card => CARD_CODE.test(card)) ||
    new Set([...board, ...hole].map(card => card.toUpperCase())).size !== 7) return undefined;
  return HAND_CATEGORY_LABELS[bestHandCategory([...hole, ...board])];
}

// Pre-`winners` fallback: the best raw hand among those shown. Kept because a
// highlight row is overwritten only by a bigger pot, so rows written before
// this field shipped stay on display for the rest of the UTC day.
function bestShownLabel(board?: string[], revealed?: Array<{name?: string; hole_cards: string[]}>) {
  const candidates = (revealed || []).filter(hand => madeHandOf(board, hand.hole_cards));
  if (candidates.length === 0) return undefined;
  const best = candidates.reduce((winner, hand) =>
    compareHands([...hand.hole_cards, ...board!], [...winner.hole_cards, ...board!]) > 0 ? hand : winner);
  const tied = candidates.filter(hand =>
    compareHands([...hand.hole_cards, ...board!], [...best.hole_cards, ...board!]) === 0);
  const names = tied.map(hand => hand.name || 'Jogador').join(' e ');
  const category = madeHandOf(board, best.hole_cards);
  return category ? `${names} — ${category}` : names;
}

export function highlightWinnerLabel(board?: string[],
  revealed?: Array<{name?: string; hole_cards: string[]}>,
  winners?: Array<{player_id: string; name?: string; payout: number}>) {
  if (!winners?.length) return bestShownLabel(board, revealed);
  const top = Math.max(...winners.map(winner => winner.payout));
  const paid = winners.filter(winner => winner.payout === top);
  const names = paid.map(winner => winner.name || 'Jogador').join(' e ');
  // Only one winner can carry a made-hand caption without ambiguity, and only
  // if they actually showed — a mucked winner has no cards to describe.
  const shown = paid.length === 1
    ? revealed?.find(hand => hand.player_id === paid[0].player_id)
    : undefined;
  const category = shown && madeHandOf(board, shown.hole_cards);
  return category ? `${names} — ${category}` : names;
}
```

Atualizar a chamada dentro do componente:

```tsx
  const revealedText = highlightWinnerLabel(data.board, data.revealed, data.winners);
```

Se `RevealedHand` de `@/lib/api/highlights` não expuser `player_id` no tipo usado aqui, ele já o
expõe (`ui/src/lib/api/highlights.ts`), então nenhuma mudança extra é necessária.

- [x] **Step 5: Rodar e confirmar que passa**

Run: `cd ui && npx vitest run src/components/table/TodayHighlight.test.tsx`
Expected: PASS, incluindo os testes pré-existentes de fallback.

- [x] **Step 6: Lint**

Run: `cd ui && npm run lint`
Expected: sem erros.

- [x] **Step 7: Commit**

```bash
git add ui/src/lib/api/highlights.ts ui/src/components/table/TodayHighlight.tsx ui/src/components/table/TodayHighlight.test.tsx
git commit -m "fix(ui): name the paid winner on the daily highlight, showdown or not"
```

---

### Task 3: Uma instância publica, as outras só sincronizam

**Files:**
- Modify: `api/internal/table/actor_views.go:122-185`
- Modify: `api/internal/table/actor_hooks.go:57-79`
- Modify: `api/internal/table/actor_presence.go:120-133`
- Test: `api/internal/table/changenotify_test.go:73-96`

**Interfaces:**
- Consumes: nada de tasks anteriores.
- Produces: `(a *Actor) syncWithoutPublish()` e `(a *Actor) publishSnapshots()`; `notifyHandComplete()` passa a devolver `bool` (rodou os hooks nesta chamada).

- [x] **Step 1: Reescrever o teste de contrato**

Em `api/internal/table/changenotify_test.go`, substituir
`TestHandleExternalChangeForcesReloadAndBroadcast` inteiro por:

```go
// ws.RedisRegistry.Broadcast PUBLISHes to Valkey and every instance delivers
// to its own local conns, so the committing instance's publish already
// reached every player wherever they are connected. A sibling republishing
// the same version sent the client a second frame carrying that sibling's own
// broadcast-time overlays (streak, equity), which the UI cannot order against
// the first — the reported badge flicker. The reload and the sweeps still
// have to run: that is what re-arms this instance's timers.
func TestHandleExternalChangeReloadsWithoutRepublishing(t *testing.T) {
	published := 0
	a := New("table-1", nil, true, func(string, hand.Snapshot) { published++ })
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)

	if err := a.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if published != 0 {
		t.Fatalf("a sibling published %d frames for a commit it did not run, want 0", published)
	}
}

// The instance that actually committed is the one that publishes, and it
// still publishes to every seat, not only to its own connections.
func TestBroadcastAllPublishesToEverySeat(t *testing.T) {
	publishedFor := map[string]bool{}
	a := New("table-1", nil, true, func(viewerID string, _ hand.Snapshot) {
		publishedFor[viewerID] = true
	})
	t.Cleanup(func() { a.afkSweepTimer.Stop() })
	a.cached = hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)

	a.broadcastAll()

	for _, id := range []string{"p1", "p2"} {
		if !publishedFor[id] {
			t.Fatalf("player %s was never published to by the committing instance", id)
		}
	}
}
```

- [x] **Step 2: Rodar e confirmar que falha**

Run: `cd api && go test ./internal/table/ -run 'TestHandleExternalChangeReloadsWithoutRepublishing|TestBroadcastAllPublishesToEverySeat' -race`
Expected: FAIL — `a sibling published 2 frames for a commit it did not run, want 0`.

- [x] **Step 3: Separar publish de sincronização**

Em `api/internal/table/actor_views.go`, substituir a função `broadcastAll` inteira (linhas 122-185)
por:

```go
// broadcastAll runs this command's sweeps and timers and then publishes one
// snapshot per seat. Only the instance that actually ran the command calls
// it — see syncWithoutPublish.
func (a *Actor) broadcastAll() { a.sync(true) }

// syncWithoutPublish is broadcastAll minus the publish: the pending-exit and
// preselection sweeps, the timer re-arming and the post-hand hooks all still
// run, but nothing goes on the wire.
//
// ws.RedisRegistry.Broadcast (api-commons/ws) PUBLISHes to a Valkey channel
// that EVERY instance is subscribed to, and each delivers to its own local
// connections. Delivery is therefore fleet-wide from a single publish: the
// instance that committed already reached every player, wherever they are
// connected. A sibling republishing the same snapshot_version sent the client
// a duplicate frame decorated with that sibling's own broadcast-time overlays
// — its 30s-paced streak map and its own Monte-Carlo equity sample — and the
// UI has no way to order two frames sharing a version, so the badge and the
// win-% visibly flipped between them.
// See docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md.
func (a *Actor) syncWithoutPublish() { a.sync(false) }

func (a *Actor) sync(publish bool) {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	// These three commit on their own, off whatever command triggered the
	// broadcast, so they need their own deadline — a Background context here
	// would leave a hung DynamoDB call pinning the actor goroutine past the
	// budget the command itself runs under (#223). The safe-completion budget,
	// not the interactive one: removeEligiblePendingExits settles a seat.
	sweepCtx, cancel := context.WithTimeout(context.Background(), a.settlementBudget)
	defer cancel()
	a.processPendingExitAutoFolds(sweepCtx)
	a.processInlinePreselections(sweepCtx)
	a.removeEligiblePendingExits(sweepCtx)
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	grace := time.Duration(0)
	if stage != a.lastBroadcastStage && isRevealStreet(stage) {
		grace = RevealGrace
	}
	a.armTurnTimer(current, stage, grace)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
	a.armWinnerCardsTimer(a.cached.PendingWinnerCards())
	a.lastBroadcastStage = stage
	if publish {
		a.publishSnapshots()
	}
	// The post-hand hooks are what compute this hand's streak badges
	// (tablemanager's onHandComplete wrapper calls SetStreaksForActor), and
	// they necessarily run after the publish above. Publishing a second time
	// when they actually ran is what puts the winner's new badge on screen at
	// the end of the hand instead of one commit late.
	if a.notifyHandComplete() && publish {
		a.publishSnapshots()
	}
}

// publishSnapshots builds and sends one viewer-scoped snapshot per seat.
func (a *Actor) publishSnapshots() {
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	// Chat and reactions are identical for every viewer, so build them once
	// per broadcast instead of once per seat (#37). Both slices are only ever
	// read downstream (ConvertSnapshot marshals them), never mutated.
	chat, reactions := a.activityViews()
	for _, p := range a.cached.PlayersForActor() {
		snapshot := a.cached.ViewFor(p.ID)
		snapshot.SnapshotVersion = uint64(a.version)
		snapshot.HandID = a.handID
		snapshot.ActionDeadlineUnixMs, snapshot.ActionBaseDeadlineUnixMs, snapshot.NextHandUnixMs =
			a.deadlinesForBroadcast(current, stage)
		if p.LastActionAt > 0 {
			snapshot.IdleRemovalUnixMs = p.LastActionAt + a.kickGrace.Milliseconds()
		}
		a.applyPresence(snapshot.Seats)
		a.applyStreaks(snapshot.Seats)
		a.applyActivity(p.ID, &snapshot, chat, reactions)
		if doEquity {
			if hole, board, ok := a.cached.HoleAndBoardForActor(p.ID); ok {
				opponents := 0
				for _, seat := range snapshot.Seats {
					if seat.PlayerID != p.ID && (seat.State == "active" || seat.State == "all_in") {
						opponents++
					}
				}
				if opponents > 0 {
					if estimate, ok := a.equityFor(hole, board, opponents); ok {
						for i := range snapshot.Seats {
							if snapshot.Seats[i].PlayerID == p.ID {
								snapshot.Seats[i].Equity = &estimate
								break
							}
						}
					}
				}
			}
		}
		a.broadcast(p.ID, snapshot)
	}
}
```

- [x] **Step 4: Fazer `notifyHandComplete` reportar se rodou**

Em `api/internal/table/actor_hooks.go`, mudar a assinatura e os returns de `notifyHandComplete`:

```go
// ... (comentário existente preservado, acrescentando a frase abaixo)
// Returns true only when this call actually ran the hooks, so sync can
// publish once more afterwards — the hooks are what produce this hand's
// streak badges.
func (a *Actor) notifyHandComplete() bool {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.completedHandNotified == a.handID {
		return false
	}
	outcome := a.cached.LastOutcomeForActor()
	if outcome == nil {
		return false
	}
	// Mark before claiming: a lost claim means another instance owns this
	// hand, and re-asking on every later broadcast of the same hand would
	// be one wasted round trip per chat message.
	a.completedHandNotified = a.handID
	if !a.claimHandHooks() {
		return false
	}
	if a.onHandComplete == nil {
		return false
	}
	names := make(map[string]string)
	for _, p := range a.cached.PlayersForActor() {
		if p.Name != "" {
			names[p.ID] = p.Name
		}
	}
	hookOutcome := *outcome
	hookOutcome.FairnessProofs = a.cached.FairnessProofsForActor()
	a.onHandComplete(a.handID, hookOutcome, names)
	return true
}
```

- [x] **Step 5: Trocar a chamada no external change**

Em `api/internal/table/actor_presence.go`, no comentário e corpo de `handleExternalChange`,
trocar a última frase do comentário e a chamada:

```go
// handleExternalChange reacts to a ChangeNotifier signal (see
// SetChangeNotifierForActor): a sibling process just committed for this
// table, so this instance forces a fresh reload — reloading also re-arms
// every timer via rearmTimersFromCache — and re-runs the per-broadcast
// sweeps. It deliberately does NOT publish: the committing instance's own
// publish is fleet-wide (see syncWithoutPublish). Always unconditional,
// unlike handleReconnect above: this only ever fires when something
// genuinely changed, never on routine local traffic.
func (a *Actor) handleExternalChange(ctx context.Context, _ ExternalChangeCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	a.syncWithoutPublish()
	return nil
}
```

- [x] **Step 6: Rodar o pacote inteiro**

Run: `cd api && go test ./internal/table/ -race`
Expected: PASS. Se algum teste pré-existente afirmar que o external change publica, ele codifica o
bug — atualize-o para a nova expectativa citando este plano no comentário.

- [x] **Step 7: Rodar tudo**

Run: `cd api && go test ./... -race`
Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add api/internal/table/actor_views.go api/internal/table/actor_hooks.go api/internal/table/actor_presence.go api/internal/table/changenotify_test.go
git commit -m "fix(table): stop sibling instances from republishing an already fleet-wide snapshot"
```

---

### Task 4: Equity determinística entre instâncias

**Files:**
- Modify: `api/internal/engine/equity/equity.go:227`
- Test: `api/internal/engine/equity/equity_test.go`

**Interfaces:**
- Consumes: `makeCacheKey(hole, board, deadCards, numOpponents, iterations) (cacheKey, bool)` já existente.
- Produces: `seedFor(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) uint64`.

- [x] **Step 1: Escrever o teste que falha**

Acrescentar ao fim de `api/internal/engine/equity/equity_test.go`:

```go
// Two API instances estimating the same spot must answer the same number.
// The sample used to be seeded from rand.Uint64(), so each process produced a
// different value for one identical (hole, board, opponents) — and because
// both instances broadcast the same snapshot_version to the same client, the
// seat's win-% visibly flipped between them (0.25, 0.19, 0.25, ...).
func TestEstimateIsIdenticalAcrossProcesses(t *testing.T) {
	hole := [2]deck.Card{{Rank: "A", Suit: "h"}, {Rank: "Q", Suit: "d"}}
	board := []deck.Card{{Rank: "A", Suit: "s"}, {Rank: "3", Suit: "s"}, {Rank: "2", Suit: "s"}}

	// Distinct tableIDs so the process-global result cache cannot be what
	// makes the two answers agree.
	first, _, err := EstimateForTableWithStats("instance-a", hole, board, nil, 2, 200)
	if err != nil {
		t.Fatalf("estimate a: %v", err)
	}
	second, _, err := EstimateForTableWithStats("instance-b", hole, board, nil, 2, 200)
	if err != nil {
		t.Fatalf("estimate b: %v", err)
	}
	if first != second {
		t.Fatalf("same spot estimated as %v and %v across instances", first, second)
	}
}

// Different spots must not collapse onto one seed and one answer.
func TestEstimateStillDiscriminatesBetweenSpots(t *testing.T) {
	board := []deck.Card{{Rank: "A", Suit: "s"}, {Rank: "3", Suit: "s"}, {Rank: "2", Suit: "s"}}
	strong, err := Estimate([2]deck.Card{{Rank: "A", Suit: "h"}, {Rank: "A", Suit: "d"}}, board, nil, 2, 2000)
	if err != nil {
		t.Fatalf("strong: %v", err)
	}
	weak, err := Estimate([2]deck.Card{{Rank: "7", Suit: "h"}, {Rank: "2", Suit: "d"}}, board, nil, 2, 2000)
	if err != nil {
		t.Fatalf("weak: %v", err)
	}
	if strong <= weak {
		t.Fatalf("AA (%v) must beat 72o (%v) on this board", strong, weak)
	}
}
```

Se `Estimate` não existir com essa assinatura no pacote, use
`EstimateWithStats(hole, board, nil, 2, 2000)` e descarte o segundo retorno — confira o topo do
arquivo antes de escrever.

- [x] **Step 2: Rodar e confirmar que falha**

Run: `cd api && go test ./internal/engine/equity/ -run TestEstimateIsIdenticalAcrossProcesses -race -count=1`
Expected: FAIL — os dois valores diferem.

- [x] **Step 3: Implementar a semente determinística**

Em `api/internal/engine/equity/equity.go`, logo acima de `EstimateForTableWithStats`:

```go
// seedFor derives the Monte-Carlo seed from the spot itself instead of the
// process's RNG. Any API instance may serve any table and several broadcast
// the same versioned snapshot to the same client, so a per-process sample
// meant the same seat's win-% arrived as two different numbers and visibly
// flipped. Deriving it here makes the estimate a pure function of its inputs,
// which is also what the result cache above has always assumed. tableID is
// deliberately NOT part of it: the estimate is table-independent, tableID
// only scopes cache eviction.
func seedFor(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) uint64 {
	const (
		offset64 = 1469598103934665603
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	mix := func(v uint64) {
		for i := 0; i < 8; i++ {
			h ^= (v >> (i * 8)) & 0xff
			h *= prime64
		}
	}
	ids := []uint8{handeval.CardID(hole[0]), handeval.CardID(hole[1])}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		mix(uint64(id))
	}
	boardIDs := make([]uint8, 0, len(board))
	for _, c := range board {
		boardIDs = append(boardIDs, handeval.CardID(c))
	}
	sort.Slice(boardIDs, func(i, j int) bool { return boardIDs[i] < boardIDs[j] })
	for _, id := range boardIDs {
		mix(uint64(id))
	}
	deadIDs := make([]uint8, 0, len(deadCards))
	for _, c := range deadCards {
		deadIDs = append(deadIDs, handeval.CardID(c))
	}
	sort.Slice(deadIDs, func(i, j int) bool { return deadIDs[i] < deadIDs[j] })
	for _, id := range deadIDs {
		mix(uint64(id))
	}
	mix(uint64(numOpponents))
	mix(uint64(iterations))
	if h == 0 {
		return 0x853c49e6748fea9b
	}
	return h
}
```

Trocar a linha 227:

```go
	rng := rng64{state: rand.Uint64()}
```

por:

```go
	rng := rng64{state: seedFor(hole, board, deadCards, numOpponents, iterations)}
```

Remover `"math/rand/v2"` do bloco de imports se ele ficar sem uso — rode
`cd api && go build ./...` para confirmar.

- [x] **Step 4: Rodar e confirmar que passa**

Run: `cd api && go test ./internal/engine/equity/ -race -count=1`
Expected: PASS, incluindo os testes de precisão pré-existentes. Se algum teste pré-existente
verificar uma margem de erro, ele continua válido: a semente mudou, não o estimador. Se uma margem
apertada falhar para uma mão específica, amplie a margem no teste, nunca o número de iterações da
produção.

- [x] **Step 5: Commit**

```bash
git add api/internal/engine/equity/equity.go api/internal/engine/equity/equity_test.go
git commit -m "fix(equity): seed the Monte-Carlo sample from the spot so every instance agrees"
```

---

### Task 5: Badge de streak fresco no fim da mão

**Files:**
- Modify: `api/internal/table/actor_views.go` (`SetStreaksForActor`)
- Modify: `api/internal/table/actor_presence.go` (`handleExternalChange`)
- Test: `api/internal/table/streakfreshness_test.go` (criar)

**Interfaces:**
- Consumes: `syncWithoutPublish()` da Task 3; `fakeStreakStore`/`streakActor` de `streakstore_test.go`; `fakeChangeNotifier` de `changenotify_test.go`.
- Produces: nenhuma API nova.

- [x] **Step 1: Escrever os testes que falham**

Criar `api/internal/table/streakfreshness_test.go`:

```go
package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// Only the instance that wins claimHandHooks publishes the new badges. Every
// other instance learns about them through the same ChangeNotifier channel it
// already uses for commits — otherwise it keeps serving the previous hand's
// numbers for up to StreakRefreshInterval, which is exactly the 6-21s windows
// the 2026-09-15 capture shows.
func TestPublishingStreaksNotifiesSiblingInstances(t *testing.T) {
	store := newFakeStreakStore()
	actor, _ := streakActor(t, store)
	notifier := newFakeChangeNotifier()
	actor.SetChangeNotifierForActor(notifier)

	actor.SetStreaksForActor(map[string]int{"p1": 2, "p2": -1})

	if got := notifier.awaitOne(t); got != "table-1" {
		t.Fatalf("notified table_id = %q, want %q", got, "table-1")
	}
}

// The sibling's side of that signal: a reload landing on a completed hand
// re-reads the badge even though the pacing window has not lapsed. Gated on
// Complete so this stays a handful of reads per hand and never one per
// command — the regression StreakRefreshInterval exists to prevent (#222).
func TestExternalChangeOnACompletedHandRefreshesTheBadge(t *testing.T) {
	fakeClock(t)
	store := newFakeStreakStore()
	actor, table := streakActor(t, store)
	actor.refreshStreaks(context.Background()) // opens the pacing window
	store.shared["p1"] = 3
	store.loads = 0
	table.StartHand()
	forceComplete(t, table)

	if err := actor.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	if store.loads != 1 {
		t.Fatalf("streak loads on a completed-hand external change = %d, want 1", store.loads)
	}
	if actor.streaks["p1"] != 3 {
		t.Fatalf("streaks = %v, want the freshly published p1=3", actor.streaks)
	}
}

// Mid-hand external changes are the common case (every action commits), so
// they must not each cost a Valkey read.
func TestExternalChangeMidHandKeepsThePacingWindow(t *testing.T) {
	fakeClock(t)
	store := newFakeStreakStore()
	actor, table := streakActor(t, store)
	actor.refreshStreaks(context.Background())
	store.loads = 0
	table.StartHand()

	for range 10 {
		if err := actor.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
			t.Fatalf("handleExternalChange: %v", err)
		}
	}

	if store.loads != 0 {
		t.Fatalf("streak loads on mid-hand external changes = %d, want 0", store.loads)
	}
}

// forceComplete drives the table to hand.Complete by folding everyone but one
// player, which is the same path an all-in nobody calls takes.
func forceComplete(t *testing.T, table *hand.Table) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for table.Stage() != hand.Complete {
		current := table.CurrentPlayerIDForActor()
		if current == "" || time.Now().After(deadline) {
			t.Fatalf("table never reached Complete, stuck at stage %v", table.Stage())
		}
		if _, err := table.Act(current, "fold", 0); err != nil {
			t.Fatalf("fold for %s: %v", current, err)
		}
	}
}
```

- [x] **Step 2: Rodar e confirmar que falha**

Run: `cd api && go test ./internal/table/ -run 'TestPublishingStreaksNotifiesSiblingInstances|TestExternalChangeOnACompletedHandRefreshesTheBadge|TestExternalChangeMidHandKeepsThePacingWindow' -race`
Expected: FAIL nos dois primeiros (nenhum notify; zero loads). Se `hand.Table` não expuser `Act` ou
`StartHand` com essas assinaturas, ajuste `forceComplete` ao que o pacote oferece — confira
`api/internal/engine/hand/hand.go` antes de escrever.

- [x] **Step 3: Notificar depois de publicar o streak**

Em `api/internal/table/actor_views.go`, no fim de `SetStreaksForActor`, trocar:

```go
	if merged != nil {
		a.streaks = merged
	}
```

por:

```go
	if merged != nil {
		a.streaks = merged
	}
	// Every other instance serving this table is still holding the previous
	// hand's badges, and refreshStreaks alone would only heal them when the
	// pacing window lapses — up to StreakRefreshInterval of a stale number on
	// the wire. This is the one moment the value actually changed, so reuse
	// the commit channel to tell them: handleExternalChange re-reads the badge
	// whenever the reload lands on a completed hand.
	a.notifyChange()
```

- [x] **Step 4: Forçar a releitura no external change**

Em `api/internal/table/actor_presence.go`, `handleExternalChange` passa a ser:

```go
func (a *Actor) handleExternalChange(ctx context.Context, _ ExternalChangeCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	// A sibling's signal may BE the hand completion that moved every badge
	// (SetStreaksForActor notifies right after publishing them), so clear the
	// pacing stamp and re-read. Gated on Complete: mid-hand commits are the
	// common case and must not each pay a Valkey round trip on the actor
	// goroutine (#222).
	if a.cached != nil && a.cached.Stage() == hand.Complete {
		a.streaksRefreshedAt = time.Time{}
		a.refreshStreaks(ctx)
	}
	a.syncWithoutPublish()
	return nil
}
```

Garantir que `hand` e `time` estejam importados em `actor_presence.go`.

- [x] **Step 5: Rodar e confirmar que passa**

Run: `cd api && go test ./internal/table/ -race`
Expected: PASS, incluindo `streakpacing_test.go` (a janela de 30 s continua valendo para comandos
fora do `Complete`).

- [x] **Step 6: Rodar tudo**

Run: `cd api && go test ./... -race`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add api/internal/table/actor_views.go api/internal/table/actor_presence.go api/internal/table/streakfreshness_test.go
git commit -m "fix(table): push the completed hand's streak badge to sibling instances"
```

---

### Task 6: Regressão de ponta a ponta e documentação

**Files:**
- Modify: `api/internal/table/crossinstance_integration_test.go`
- Modify: `docs/README.md`

**Interfaces:**
- Consumes: tudo acima.
- Produces: nada.

- [x] **Step 1: Escrever o teste de regressão cross-instance**

Ler `api/internal/table/crossinstance_integration_test.go` inteiro primeiro para reaproveitar os
helpers de duas instâncias que ele já tem, e acrescentar:

```go
// The 2026-09-15 capture: the same (hand_id, snapshot_version) reached one
// client three times, and the only fields that ever differed between those
// frames were current_streak and equity — both broadcast-time overlays owned
// by the emitting instance. Whatever two instances publish for one version
// has to be identical now.
func TestTwoInstancesNeverPublishDivergentFramesForOneVersion(t *testing.T) {
	// ... monta duas instâncias sobre o mesmo store e o mesmo fakeStreakStore,
	// coleta cada snapshot publicado em map[uint64][]hand.Snapshot por viewer,
	// roda uma mão até Complete e falha se dois snapshots com a mesma
	// SnapshotVersion diferirem em Seats[i].CurrentStreak ou Seats[i].Equity.
}
```

Escreva o corpo com os helpers reais do arquivo; a asserção obrigatória é: para cada viewer e cada
`SnapshotVersion`, todos os snapshots coletados têm o mesmo `CurrentStreak` e o mesmo `Equity` em
cada assento.

- [x] **Step 2: Rodar**

Run: `cd api && go test ./internal/table/ -run TestTwoInstancesNeverPublishDivergentFramesForOneVersion -race -count=5`
Expected: PASS de forma estável nas 5 execuções.

- [x] **Step 3: Registrar no índice de docs**

Acrescentar uma linha em `docs/README.md`, na seção de specs implementados, apontando para
`docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md` e resumindo: highlight
passa a persistir o vencedor; instâncias irmãs não republicam; equity e streak convergem.

- [x] **Step 4: Verificação final**

Run: `cd api && go test ./... -race && cd ../ui && npm test && npm run lint`
Expected: tudo PASS.

- [x] **Step 5: Commit**

```bash
git add api/internal/table/crossinstance_integration_test.go docs/README.md
git commit -m "test(table): pin identical frames per snapshot version across instances"
```

---

## Verificação do sintoma 3 (cartas incompletas)

Não há task de correção: a causa raiz não foi provada e o servidor está inocentado pela evidência do
spec §2.3. Depois que as Tasks 3–5 estiverem em produção, a carga de decode+render por versão cai
~3×. Para revalidar é preciso:

1. Um HAR **sem filtro de tipo** (incluindo assets), capturando uma partida inteira — permite ver se
   alguma face SVG falhou no fetch.
2. Ou um performance trace do Chrome no momento da falha — mostra se a animação de entrada ficou
   `pending` com o card em `opacity: 0`.

Se a captura mostrar o card parado em `opacity: 0` com `data-card-revealing` presente além de
`CARD_REVEAL_MS`, a investigação continua em `ui/src/lib/hooks/useEnteredKeys.ts`. Se mostrar um
fetch de face falhando, continua em `PlayingCard`'s `faceBroken`. Não mexa em nenhum dos dois antes
de ter essa evidência.

---

## Execution notes (2026-09-17)

Executado inteiro no branch `fix/table-snapshot-divergence-and-highlight-winner`. Desvios do plano,
todos deliberados:

- **Task 2** também atualizou `ui/src/app/(marketing)/guide/table/page.tsx` — a verba "Maior pote de
  hoje" descrevia o comportamento antigo ("com a mão vencedora quando ela foi revelada"), e
  `ui/CLAUDE.md` exige a atualização do guia no mesmo change.
- **Task 3** precisou também de `internal/tablemanager/changelisten_test.go`:
  `TestListenForExternalChangesDispatchesToTheMatchingLocalActor` afirmava o comportamento antigo.
  Reescrito para observar o dispatch pela ordem do mailbox (`Dispatch` bloqueia até a resposta) em vez
  do broadcast, e para falhar se um frame for publicado.
- **Task 5** reaproveita o helper `completedTable` que já existia em `audit_regression_test.go` em vez
  de criar `forceComplete`.
- **Task 6** virou um teste de unidade (`internal/table/framedivergence_test.go`) em vez de entrar em
  `crossinstance_integration_test.go`: aquele arquivo é `//go:build integration` e precisa de DynamoDB
  Local, então o teste não rodaria no gate normal — que é exatamente onde esta regressão precisa ser
  pega. Verificado que ele falha com a correção da Task 5 revertida
  (`streak 1 from one instance, 0 from the other`).
- O fixture da Task 6 exercita a metade do streak; o determinismo da equity entre processos fica
  pinado por `TestEstimateIsIdenticalAcrossProcesses` em `internal/engine/equity`.

`api/CLAUDE.md` ganhou a regra "Only the instance that ran the command publishes" e teve duas
afirmações desatualizadas corrigidas (`handleExternalChange` já não chama `broadcastAll`).

Gate final: `go test ./... -race` e `go vet -tags integration ./...` verdes; `npx vitest run`
(1687 testes), `npx tsc --noEmit` e `npx eslint src --max-warnings 0` verdes.
