# Countdown de 5s entre mãos + auto-continuação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Depois que uma mão chega em `Complete`, a mesa espera 5 segundos (visíveis a todos via um
indicador de contagem regressiva) e então inicia a próxima mão automaticamente — sem qualquer ação do
cliente. Isso fecha uma lacuna real deixada pela Feature A: o Task 6 daquele plano removeu o botão
manual "Estou pronto", e hoje **nada** dispara `tryStartHand()` depois que uma mão termina, exceto um
novo `ReadyCmd`/`JoinCmd` — ou seja, sem esta feature a mesa trava em `complete` para sempre depois da
primeira mão em produção.

**Architecture:** `hand.Table` (engine puro) não muda — `StartHand`/`tryStartHand` já existem e já
toleram "menos de 2 prontos" como no-op silencioso (`StartHand`'s erro é engolido em `tryStartHand`,
`api/internal/table/actor.go:266-272`). Toda a lógica nova vive em `table.Actor`, seguindo exatamente o
mesmo padrão já estabelecido pelo timer de turno unificado (`armTurnTimer`/`turnTimeoutCmd`/
`handleTurnTimeout`, Feature A Task 5) e pela escalada de blind (`internal/table/escalation.go`): um
`time.AfterFunc` que só despacha um comando pro loop único do ator (`a.Dispatch`), nunca muta estado
direto da goroutine do timer. O timer de próxima-mão é armado dentro de `broadcastAll` (mesmo lugar que
já arma o `turnTimer`), condicionado a `stage == Complete`, e é idempotente por `handID` (uma vez
armado para um `handID`, não rearma de novo até o `handID` mudar — evita reiniciar o contador de 5s a
cada broadcast).

**Tech Stack:** Go (`internal/table`, `internal/engine/hand`), Next.js 16 (`useTableRealtime.ts`,
CSS-driven animations, mesmo padrão `key={value}` + `animationDuration` já usado pelo anel de turno).

## Global Constraints

- `internal/engine/hand` continua sem import de `time`/rede — o delay de 5s vive só em `table.Actor`.
- `go test ./... -race`; testes com `//go:build integration` precisam do DynamoDB Local
  (`docker-compose.test.yml`).
- UI: `eslint src --max-warnings 0` && `next build` sem erros/warnings. Nada de `setInterval`/estado
  ticking em efeito — reusar a técnica já validada (CSS anima, o componente só calcula a duração uma
  vez a partir de `deadlineMs - nowMs`, ambos vindos de props/snapshot, nunca de `Date.now()` chamado
  durante o render).

---

### Task 1: Timer universal de próxima-mão em `table.Actor`

**Files:**

- Modify: `../../api/internal/table/actor.go`
- Modify: `../../api/internal/table/commands.go`
- Modify: `../../api/internal/engine/hand/snapshot.go`
- Test: new `../../api/internal/table/nexthand_test.go`

**Interfaces:**

- `hand.Snapshot` gains `NextHandUnixMs int64 \`json:"next_hand_unix_ms,omitempty"\`` — populado por
  `Actor`, igual `ActionDeadlineUnixMs`.
- New const `NextHandDelay = 5 * time.Second` em `internal/table/turntimeout.go` (mesmo arquivo que já
  tem `DefaultTurnTimeout`, mesma convenção).
- `Actor` ganha `nextHandTimer *time.Timer`, `nextHandDeadline time.Time`, `nextHandArmedFor string`
  (guarda o `handID` para o qual o timer já foi armado — chave de idempotência, análoga a
  `turnDeadlineFor`).
- New command `nextHandCmd{Reply chan error}` (sem `PlayerID` — é uma transição de mesa inteira, não
  de um jogador).
- New `func (a *Actor) armNextHandTimer()` chamado de dentro de `broadcastAll`, ao lado de
  `armTurnTimer`.
- New `func (a *Actor) handleNextHand(ctx context.Context, c nextHandCmd) error`.

- [ ] **Step 1: Escrever os testes que falham**

Add to `internal/table/turntimeout.go`:

```go
const NextHandDelay = 5 * time.Second
```

Add `../../api/internal/table/nexthand_test.go` (sem build tag — unit puro, mesmo padrão bare-`&Actor{}`
de `turntimeout_test.go`):

```go
package table

import (
	"testing"
	"time"
)

func TestArmNextHandTimerEnqueuesNextHandCmdWhenComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Millisecond, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)

	select {
	case cmd := <-a.cmds:
		if _, ok := cmd.(nextHandCmd); !ok {
			t.Fatalf("got command %T, want nextHandCmd", cmd)
		}
		cmd.reply() <- nil
	case <-time.After(200 * time.Millisecond):
		t.Fatal("next-hand timer did not enqueue nextHandCmd")
	}
}

func TestArmNextHandTimerIsIdempotentForTheSameHandID(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	first := a.nextHandDeadline
	a.armNextHandTimer(true) // same handID — must not restart the countdown
	if !a.nextHandDeadline.Equal(first) {
		t.Fatal("re-arming for the same handID must not restart the 5s countdown")
	}
}

func TestArmNextHandTimerClearsWhenNotComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	a.armNextHandTimer(false)
	if a.nextHandArmedFor != "" {
		t.Fatal("expected nextHandArmedFor cleared once the hand is no longer Complete")
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/table/... -run TestArmNextHandTimer -v
```

Esperado: FAIL — `nextHandDelay`/`nextHandArmedFor`/`armNextHandTimer`/`nextHandCmd` não existem ainda.

- [ ] **Step 3: Adicionar o comando em `commands.go`**

```go
// nextHandCmd is dispatched by the 5s post-hand timer (a time.AfterFunc
// goroutine) so the actual StartHand attempt happens inside Run, never from
// the timer goroutine (see armNextHandTimer). A stale command (the table is
// no longer Complete, or a new hand already started through some other path)
// is a silent no-op — handleNextHand re-checks the stage before acting.
type nextHandCmd struct {
	Reply chan error
}

func (c nextHandCmd) reply() chan error { return c.Reply }
```

- [ ] **Step 4: Campos no `Actor`, `New`, e o switch de `handle`**

Em `actor.go`, junto dos outros campos de timer:

```go
	nextHandTimer    *time.Timer
	nextHandDeadline time.Time
	nextHandArmedFor string
	nextHandDelay    time.Duration
```

Em `New`, junto de `turnTimeout: DefaultTurnTimeout,`:

```go
		nextHandDelay:                NextHandDelay,
```

No switch de `handle`:

```go
	case nextHandCmd:
		return a.handleNextHand(ctx, c)
```

- [ ] **Step 5: `armNextHandTimer` e `handleNextHand`**

```go
// armNextHandTimer (re-)arms the 5s post-hand countdown when the table is
// Complete. Idempotent per handID: re-arming for the SAME hand does not
// restart the countdown (matches armTurnTimer's convention). complete is
// passed in by broadcastAll (already knows the current stage) so this stays
// a plain bool check, no engine dependency beyond what's already cached.
func (a *Actor) armNextHandTimer(complete bool) {
	if !complete {
		if a.nextHandTimer != nil {
			a.nextHandTimer.Stop()
		}
		a.nextHandArmedFor = ""
		return
	}
	if a.handID == a.nextHandArmedFor {
		return
	}
	if a.nextHandTimer != nil {
		a.nextHandTimer.Stop()
	}
	a.nextHandArmedFor = a.handID
	a.nextHandDeadline = timeNowFunc().Add(a.nextHandDelay)
	a.nextHandTimer = time.AfterFunc(a.nextHandDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(nextHandCmd{Reply: reply})
	})
}

// handleNextHand attempts to start the next hand once the 5s post-hand
// countdown expires. A stale timer (a client already returned from sitting
// out and tryStartHand already ran, or the table isn't Complete anymore) is a
// silent no-op. tryStartHand itself already swallows "fewer than 2 ready
// players" — if that happens here, the table just stays Complete until a
// ReadyCmd(true) brings it back (Feature A, Task 3), same as before this
// feature existed.
func (a *Actor) handleNextHand(ctx context.Context, c nextHandCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if a.cached.Stage() != hand.Complete {
		return nil
	}
	a.tryStartHand()
	if err := a.commit(ctx, "", nil); err != nil && !errors.Is(err, tablestore.ErrVersionConflict) {
		return err
	}
	a.broadcastAll()
	return nil
}
```

- [ ] **Step 6: Ligar em `broadcastAll` e stampar o snapshot**

```go
func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	a.armTurnTimer(current)
	a.armNextHandTimer(stage == hand.Complete)
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	for _, p := range a.cached.PlayersForActor() {
		snapshot := a.cached.ViewFor(p.ID)
		if current != "" && current == a.turnDeadlineFor {
			snapshot.ActionDeadlineUnixMs = a.turnDeadline.UnixMilli()
		}
		if stage == hand.Complete && a.handID == a.nextHandArmedFor {
			snapshot.NextHandUnixMs = a.nextHandDeadline.UnixMilli()
		}
		a.applyPlayerNames(snapshot.Seats)
		// ... unchanged (equity block)
		a.broadcast(p.ID, snapshot)
	}
}
```

Em `snapshot.go`, adicionar ao `Snapshot`:

```go
	NextHandUnixMs int64 `json:"next_hand_unix_ms,omitempty"`
```

- [ ] **Step 7: Rodar os testes puros**

```bash
cd api && go test ./internal/table/... -run TestArmNextHandTimer -v
```

- [ ] **Step 8: Teste de integração ponta a ponta**

Add to `../../api/internal/table/nexthand_integration_test.go` (`//go:build integration`, mesmo padrão
de `disconnect_test.go`):

```go
func TestHandCompleteAutoStartsNextHandAfterDelay(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)
	a.nextHandDelay = 20 * time.Millisecond

	ctx := context.Background()
	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	// Fold p1 (or whoever must act) to force the hand straight to Complete.
	stored, _ := store.LoadTable(ctx, "table-1")
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3})

	stored, _ = store.LoadTable(ctx, "table-1")
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected hand to reach Complete after fold-to-one, got %v", stored.State.Stage)
	}
	handIDAfterFold := stored.HandID

	time.Sleep(50 * time.Millisecond)

	stored, _ = store.LoadTable(ctx, "table-1")
	if stored.HandID == handIDAfterFold {
		t.Fatal("expected a new hand to have started automatically after the 5s (here 20ms) delay")
	}
}
```

- [ ] **Step 9: Rodar tudo**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/table/... -run TestHandCompleteAutoStartsNextHand -v
cd api && go test ./... -race
cd api && go test -tags integration ./... -race
```

- [ ] **Step 10: Commit**

```bash
git add api/internal/table/actor.go api/internal/table/commands.go api/internal/table/turntimeout.go \
        api/internal/table/nexthand_test.go api/internal/table/nexthand_integration_test.go \
        api/internal/engine/hand/snapshot.go
git commit -m "feat(api): auto-start the next hand 5s after the previous one completes"
```

---

### Task 2: UI — indicador de contagem regressiva entre mãos

**Files:**

- Modify: `../../ui/src/lib/api/table.ts`
- Modify: `../../ui/src/app/table/page.tsx`
- Modify: `../../ui/src/app/globals.css`
- Modify: `../../ui/src/lib/mock.ts`

**Interfaces:**

- `TableSnapshot.next_hand_unix_ms?: number` (campo novo do wire).
- Reaproveita o padrão já validado do anel de turno (Feature A Task 6): `key={value}` +
  `animationDuration` calculado uma vez por render a partir de `deadlineMs - nowMs` (ambos vindos de
  props/snapshot — `nowMs` já existe como `rt.snapshotAt`, exportado pelo hook desde a Feature A).

- [ ] **Step 1: Campo TS**

```ts
  next_hand_unix_ms?: number
```

- [ ] **Step 2: Banda "Mão encerrada" ganha o anel**

Em `page.tsx`, a banda que já existe (Feature A Task 6) ganha um indicador visual ao lado do texto,
só quando `s.next_hand_unix_ms` está presente:

```tsx
      {!connectionMessage && (s.stage === 'waiting_for_players' || s.stage === 'complete') && <div className="reconnect-notice">
          <p>{s.stage === 'complete' ? 'Mão encerrada.' : 'Aguardando jogadores.'}</p>
          {s.next_hand_unix_ms &&
            <span key={s.next_hand_unix_ms} className="next-hand-ring"
                  style={{animationDuration: `${Math.max(0, s.next_hand_unix_ms - rt.snapshotAt)}ms`}}
                  aria-hidden="true"/>}
          {viewerSeat?.state === 'sitting_out' &&
            <Button type="button" variant="ghost" onClick={() => rt.ready(true)}>Voltar a jogar</Button>}
      </div>}
```

> Nota pro implementador: se o design quiser um número regressivo em segundos em vez de (ou junto com)
> o anel, isso exige um `setInterval`/tick de estado — o que este plano evita deliberadamente (mesma
> razão documentada na Feature A: preferir CSS-only a estado ticking em efeito). Se o produto pedir o
> número, isso é um refinamento visual sobre o mesmo dado (`next_hand_unix_ms`), não uma mudança de
> arquitetura.

- [ ] **Step 3: CSS**

Ao lado de `.seat-turn-ring` em `globals.css`:

```css
.next-hand-ring {
    display: inline-block;
    width: 14px;
    height: 14px;
    margin-left: .5em;
    border-radius: 50%;
    border: 2px solid transparent;
    border-top-color: var(--gold);
    animation: seat-turn-countdown linear forwards
}
```

(Reaproveita o `@keyframes seat-turn-countdown` já criado na Feature A — mesma animação, elemento
diferente.)

- [ ] **Step 4: Mock**

Em `mock.ts`, adicionar um cenário (ou estender o existente `complete`, se houver) com
`next_hand_unix_ms: Date.now() + 5000` para poder visualizar em `?scenario=complete`.

- [ ] **Step 5: Lint, build, verificação manual**

```bash
cd ui && npx eslint src --max-warnings 0 && npx next build
```

Abrir a mesa após uma mão terminar (dois navegadores/abas) e confirmar: o anel aparece por ~5s e a
próxima mão começa sozinha, sem clique.

- [ ] **Step 6: Commit**

```bash
git add ui/src/lib/api/table.ts ui/src/app/table/page.tsx ui/src/app/globals.css ui/src/lib/mock.ts
git commit -m "feat(ui): visual countdown before the next hand auto-starts"
```

## Self-Review Notes

- **Por que esta feature existe de verdade, não é só polish:** confirmado por leitura de código (não
  suposição) que depois do Task 6 da Feature A (remoção do botão manual "Estou pronto"), não existe
  NENHUM outro gatilho de `tryStartHand()` além de `ReadyCmd`/`JoinCmd` — sem esta feature, toda mesa
  trava em `complete` após a primeira mão em produção assim que a Feature A for implantada sozinha.
- **Reaproveita 100% dos padrões já validados na Feature A** (timer via `time.AfterFunc` despachando
  comando pro loop do ator, idempotência por chave, stamp de deadline em `broadcastAll`, anel CSS via
  `key`-remount) — nenhum mecanismo novo, só uma segunda instância do mesmo mecanismo.
- **Fora de escopo, não tocado por este plano:** Feature C (reveal voluntário/sons/texto de tipo de
  mão) e Feature D (histórico/auditoria), conforme ordenação confirmada pelo usuário (bug3 → A → B → C
  → D).
