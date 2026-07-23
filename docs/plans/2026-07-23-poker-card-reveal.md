# Reveal voluntário de cartas + texto do tipo de mão + sons

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** (1) Depois que uma mão chega em `Complete`, qualquer jogador que foi dealt in nessa mão pode
voluntariamente revelar suas hole cards a todos na mesa (mesmo quando a mão terminou por fold-to-one,
sem showdown genuíno — hoje `ViewFor` nunca revela essas cartas, de propósito, por causa da correção
do "bug3" desta sessão). (2) Qualquer conjunto de cartas revelado (por showdown genuíno OU por reveal
voluntário) ganha um texto do tipo de mão (ex.: "Dois pares"), sempre que o board tiver as 5 cartas
comunitárias (sem elas não há avaliação de 7 cartas possível). (3) Efeitos sonoros usam os arquivos
`.mp3` que **já existem** em `ui/public/sounds/` (`revealing-card-table.mp3`,
`player-showing-card.mp3`, `cards-on-table.mp3`, `half-pot-chips.mp3`, `all-in-chips.mp3`,
`basic-chips-1.mp3`, `basic-chips-2.mp3`) — hoje nenhum código no client toca esses arquivos.

**Architecture:** `hand.Table` ganha um novo campo por jogador (`Player.VoluntarilyShown bool`,
persistido do mesmo jeito que `Player.Ready`/`Player.State` já são — direto no struct, sem tocar
`state.go`) e um método novo `RevealHoleCards(playerID string) error`. `ViewFor`'s condição de reveal
por seat passa a ser `dealtIn[p.ID] && (p.ID == viewerID || (revealAll && p.State != Folded) ||
p.VoluntarilyShown)` — o reveal voluntário é ortogonal ao `revealAll` da correção do bug3 (não a
substitui, soma a ela). O tipo de mão vira um campo **por seat** (`SeatView.HandCategory`), não um
campo único de mesa: em qualquer showdown com 2+ mãos reveladas cada jogador tem seu próprio tipo de
mão (o perdedor pode ter "Par", o vencedor "Dois pares") — um único campo table-wide perderia essa
informação. `table.Actor` ganha um comando novo (`ShowCardsCmd`) seguindo exatamente o padrão de
`SitOutCmd`/`handleSitOut` (ensureLoaded → apply no cache → retry-on-conflict → commit → broadcastAll).
Sons ficam inteiramente no client: um módulo novo `ui/src/lib/sound.ts` que mapeia nome de evento →
arquivo, disparado de dentro do `receive` callback do `useTableRealtime.ts` (evento, não render —
`new Audio(...).play()` é uma chamada impura, tem que rodar fora do corpo do componente).

**Tech Stack:** Go (`internal/engine/hand`, `internal/engine/handeval`, `internal/table`), Next.js 16
(`useTableRealtime.ts`, `Seat.tsx` — o flip de carta em `PlayingCard.tsx` **já existe e já é
reaproveitável sem mudança**, confirmado por leitura).

## Global Constraints

- `internal/engine/hand` continua sem import de `time`/rede.
- Valores de wire continuam em inglês/`snake_case` (ex.: `high_card`, `two_pair`) — a tradução pro
  PT-BR acontece no client, mesma convenção já usada por `STAGE_LABELS`/`STATE_LABELS` em
  `page.tsx`/`Seat.tsx`. Não traduzir no servidor.
- `go test ./... -race`; testes com `//go:build integration` precisam do DynamoDB Local.
- UI: `eslint src --max-warnings 0` && `next build` sem erros/warnings. Nenhuma biblioteca nova — Web
  Audio API nativa (`new Audio(...)`), sem dependência.

---

### Task 1: `HandCategory` por seat na engine

**Files:**

- Modify: `../../api/internal/engine/hand/snapshot.go`
- Test: `../../api/internal/engine/hand/snapshot_test.go`

**Interfaces:**

- `SeatView` ganha `HandCategory string \`json:"hand_category,omitempty"\`` — populado em `ViewFor`
  para qualquer seat cujas `HoleCards` estejam visíveis ao viewer (própria mão, `revealAll`, ou reveal
  voluntário — ver Task 2) **e** `len(t.board) == 5`. Sem essas 5 cartas não há 7-card hand a avaliar
  (ex.: fold no pré-flop revelado voluntariamente mostra só as cartas, sem rótulo de tipo).

- [ ] **Step 1: Escrever o teste que falha**

Add to `snapshot_test.go`:

```go
func TestViewForIncludesHandCategoryWhenBoardIsComplete(t *testing.T) {
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
	for _, s := range view.Seats {
		if s.HandCategory == "" {
			t.Fatalf("expected a hand_category for seat %s once the board is complete and cards are revealed", s.PlayerID)
		}
	}
}

func TestViewForOmitsHandCategoryWhenCardsAreHidden(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if s.PlayerID == "p2" && s.HandCategory != "" {
			t.Fatal("must not leak an opponent's hand category before their cards are visible")
		}
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/engine/hand/... -run TestViewFor.*HandCategory -v
```

- [ ] **Step 3: Implementar**

Em `snapshot.go`, adicionar ao `SeatView`:

```go
	HandCategory string `json:"hand_category,omitempty"`
```

Import `"gopkg.aoctech.app/poker/api/internal/engine/handeval"` (já usado por `hand.go`, mesmo pacote,
`categoryNames` já existe como var package-level em `hand.go`).

Em `ViewFor`, depois de decidir `sv.HoleCards`:

```go
		if dealtIn[p.ID] && (p.ID == viewerID || (revealAll && p.State != Folded) || p.VoluntarilyShown) {
			sv.HoleCards = []string{cardCode(p.HoleCards[0]), cardCode(p.HoleCards[1])}
			if len(t.board) == 5 {
				var full [7]deck.Card
				full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
				copy(full[2:], t.board)
				sv.HandCategory = categoryNames[handeval.Best7(full).Category()]
			}
		}
```

(A cláusula `p.VoluntarilyShown` já está aqui adiantada — o campo em si é criado na Task 2; adicionar
`VoluntarilyShown bool` ao `Player` struct agora mesmo, na Task 1, evita quebrar a compilação —
`dynamodbav:"voluntarily_shown"`.)

- [ ] **Step 4: Rodar e confirmar passagem**

```bash
cd api && go test ./internal/engine/hand/... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/snapshot.go api/internal/engine/hand/snapshot_test.go api/internal/engine/hand/hand.go
git commit -m "feat(api): expose each revealed seat's hand type (hand_category) on the wire"
```

---

### Task 2: Reveal voluntário na engine

**Files:**

- Modify: `../../api/internal/engine/hand/hand.go`
- Test: `../../api/internal/engine/hand/hand_test.go`

**Interfaces:**

- `Player.VoluntarilyShown bool` (já adicionado na Task 1) — resetado a `false` para todos no início de
  `StartHand`, junto dos outros campos por-mão (`p.Contributed = 0`, etc., dentro do loop que monta
  `active`).
- New `func (t *Table) RevealHoleCards(playerID string) error` — erro se a mesa não está `Complete`, se
  o jogador não foi dealt in nesta mão (`!dealtIn`), ou se o jogador não existe. Idempotente: chamar
  duas vezes não é erro.

- [ ] **Step 1: Escrever os testes que falham**

```go
func TestRevealHoleCardsMakesFoldedWinnerCardsVisible(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := table.playerToActForTest()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	if err := table.Act(toAct, betting.ActionFold, 0); err != nil {
		t.Fatalf("%s folds: %v", toAct, err)
	}

	if err := table.RevealHoleCards(winnerID); err != nil {
		t.Fatalf("RevealHoleCards: %v", err)
	}
	view := table.ViewFor(toAct)
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && len(s.HoleCards) != 2 {
			t.Fatal("expected the voluntarily-revealed winner's hole cards to be visible to everyone")
		}
	}
}

func TestRevealHoleCardsRejectsPlayerNotDealtIntoTheHand(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	p3 := &Player{ID: "p3", Stack: 1000}
	_ = table.AddMidHandJoiner(p3)
	if err := table.RevealHoleCards("p3"); err == nil {
		t.Fatal("expected an error revealing cards for a player never dealt into this hand")
	}
}

func TestVoluntarilyShownResetsOnNextHand(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	toAct := table.playerToActForTest()
	_ = table.Act(toAct, betting.ActionFold, 0)
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	_ = table.RevealHoleCards(winnerID)

	if err := table.StartHand(); err != nil {
		t.Fatalf("second StartHand: %v", err)
	}
	if table.playerByID(winnerID).VoluntarilyShown {
		t.Fatal("VoluntarilyShown must reset at the start of the next hand")
	}
}
```

- [ ] **Step 2: Rodar e confirmar falha**

```bash
cd api && go test ./internal/engine/hand/... -run "TestRevealHoleCards|TestVoluntarilyShownResets" -v
```

- [ ] **Step 3: Implementar**

Em `hand.go`, adicionar ao `Player` struct (se ainda não feito na Task 1):

```go
	VoluntarilyShown bool `dynamodbav:"voluntarily_shown"`
```

No loop ativo de `StartHand` (o mesmo loop que já zera `p.Contributed = 0` para quem entra `active`),
adicionar a mesma linha para **todos** os jogadores da mesa, não só os `active` — um jogador que ficou
de fora (sitting-out) também precisa ter a flag limpa para a mão seguinte:

```go
	for _, p := range t.players {
		p.VoluntarilyShown = false
	}
```

(Colocar essa linha logo antes do loop existente que monta `active`, não dentro dele — precisa cobrir
todo mundo, inclusive quem não joga esta mão.)

Adicionar, perto de `RequestReturnFromSitOut`:

```go
// RevealHoleCards lets a player who was dealt into the just-completed hand
// voluntarily show their cards to everyone, even when the hand ended without
// a genuine showdown (fold-to-one) — ViewFor's revealAll gate never covers
// this case on purpose (see the bug3 fix), so this is a separate, per-player
// opt-in. Idempotent: calling it twice for the same player is a no-op, not an
// error.
func (t *Table) RevealHoleCards(playerID string) error {
	if t.stage != Complete {
		return fmt.Errorf("hand: cards can only be revealed after the hand is complete")
	}
	dealtIn := false
	for _, hp := range t.handOrder {
		if hp.ID == playerID {
			dealtIn = true
			break
		}
	}
	if !dealtIn {
		return fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
	}
	t.playerByID(playerID).VoluntarilyShown = true
	return nil
}
```

- [ ] **Step 4: Rodar e confirmar passagem**

```bash
cd api && go test ./internal/engine/hand/... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/hand/hand.go api/internal/engine/hand/hand_test.go
git commit -m "feat(api): voluntary post-hand card reveal (RevealHoleCards)"
```

---

### Task 3: Comando `ShowCardsCmd` no `table.Actor`

**Files:**

- Modify: `../../api/internal/table/commands.go`
- Modify: `../../api/internal/table/actor.go`
- Test: `../../api/internal/table/actor_test.go` (integration)

**Interfaces:**

- New `ShowCardsCmd{PlayerID string, Reply chan error}`, seguindo exatamente o padrão de `SitOutCmd`/
  `handleSitOut`.

- [ ] **Step 1: Comando**

```go
type ShowCardsCmd struct {
	PlayerID string
	Reply    chan error
}

func (c ShowCardsCmd) reply() chan error { return c.Reply }
```

- [ ] **Step 2: Handler**

```go
	case ShowCardsCmd:
		return a.handleShowCards(ctx, c)
```

```go
func (a *Actor) handleShowCards(ctx context.Context, c ShowCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if err := a.cached.RevealHoleCards(c.PlayerID); err != nil {
			return err
		}
		return a.commit(ctx, "", nil)
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}
```

- [ ] **Step 3: Teste de integração**

```go
func TestShowCardsCmdRevealsFoldedWinnerToEveryone(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)
	ctx := context.Background()

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(ctx, "table-1")
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3}); err != nil {
		t.Fatalf("fold: %v", err)
	}

	reply4 := make(chan error, 1)
	if err := a.Dispatch(ShowCardsCmd{PlayerID: winnerID, Reply: reply4}); err != nil {
		t.Fatalf("ShowCardsCmd: %v", err)
	}
	stored, _ = store.LoadTable(ctx, "table-1")
	table := hand.NewTableFromState(stored.State)
	view := table.ViewFor(toAct)
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && len(s.HoleCards) != 2 {
			t.Fatal("expected winner's cards visible to the other player after ShowCardsCmd")
		}
	}
}
```

- [ ] **Step 4: Rodar tudo**

```bash
docker compose -f docker-compose.test.yml up -d
cd api && go test -tags integration ./internal/table/... -run TestShowCardsCmd -v
cd api && go test ./... -race && go test -tags integration ./... -race
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/table/commands.go api/internal/table/actor.go api/internal/table/actor_test.go
git commit -m "feat(api): wire ShowCardsCmd for voluntary post-hand card reveal"
```

---

### Task 4: UI — botão "Mostrar cartas" + texto do tipo de mão

**Files:**

- Modify: `../../ui/src/lib/api/table.ts`
- Modify: `../../ui/src/lib/hooks/useTableRealtime.ts`
- Modify: `../../ui/src/app/table/page.tsx`
- Modify: `../../ui/src/components/table/Seat.tsx`

**Interfaces:**

- `SeatView.hand_category?: string` (novo campo do wire).
- `useTableRealtime` ganha `showCards: () => void` no objeto retornado, emitindo
  `{type: 'show_cards'}` (verificar o nome exato do tipo de mensagem que o gateway espera — checar
  `tablews.go` / o switch de tipos de mensagem do servidor antes de fixar o nome; este plano assume
  `show_cards` por analogia com `ready`/`act`, mas o implementador deve confirmar contra o código do
  gateway WS antes de codar).
- Tradução PT-BR de `hand_category`: novo `const HAND_CATEGORY_LABELS: Record<string, string>` em
  `Seat.tsx` (mesmo padrão de `STATE_LABELS`), cobrindo os 10 valores de `categoryNames` em
  `internal/engine/hand/hand.go` (`high_card` → "Carta alta", `pair` → "Par", `two_pair` → "Dois
  pares", `three_of_a_kind` → "Trinca", `straight` → "Sequência", `flush` → "Flush", `full_house` →
  "Full house", `four_of_a_kind` → "Quadra", `straight_flush` → "Straight flush", `royal_flush` →
  "Royal flush").

- [ ] **Step 1-N (resumo — seguir o padrão exato do Task 6 da Feature A):**
  1. Campo TS em `table.ts`.
  2. `useTableRealtime.ts`: adicionar `showCards` ao objeto retornado, análogo a `ready`/`sendChat`.
  3. `Seat.tsx`: renderizar `HAND_CATEGORY_LABELS[seat.hand_category] ` como uma `<small>` extra
     (mesma posição visual de `seat-state`) quando `seat.hand_category` estiver presente.
  4. `page.tsx`: quando `s.stage === 'complete'` e o seat do viewer tem `hole_cards` vazio (ainda não
     revelado) e o viewer foi dealt in nesta mão (checar via alguma pista disponível no snapshot — se
     necessário, adicionar um booleano auxiliar no wire, mas preferir primeiro checar se
     `viewerSeat.state` já distingue isso antes de adicionar campo novo), mostrar um botão "Mostrar
     cartas" chamando `rt.showCards()`.
  5. Lint + build: `npx eslint src --max-warnings 0 && npx next build`.
  6. Commit: `feat(ui): voluntary "show cards" button and hand-type label on reveal`.

> Nota pro implementador: o passo 4 precisa de um jeito de saber, no client, se o viewer tem uma mão
> "escondível" nesta rodada (dealt in, mas cartas ainda não em `hole_cards` do próprio snapshot — o
> viewer sempre vê a própria mão via `p.ID == viewerID` em `ViewFor`, então isso não serve de sinal).
> O sinal certo é: `s.stage === 'complete'` e a mão terminou sem showdown genuíno — o client não tem
> essa informação hoje (`won_without_showdown` não está no wire). Antes de codar esta task, decidir se
> vale adicionar `Snapshot.WonWithoutShowdown bool` (mapeando `HandOutcome.WonWithoutShowdown`, hoje
> server-internal) — é o sinal mais direto e evita heurísticas frágeis no client.

---

### Task 5: Sons

**Files:**

- New: `../../ui/src/lib/sound.ts`
- Modify: `../../ui/src/lib/hooks/useTableRealtime.ts`

**Interfaces:**

- `playSound(name: SoundName): void` — `new Audio(path).play().catch(() => {})` (o `.catch` engole o
  erro comum de autoplay bloqueado por política do navegador antes de qualquer interação do usuário;
  não é um erro real da aplicação).
- `type SoundName = 'reveal' | 'showing_card' | 'dealing' | 'half_pot' | 'all_in' | 'bet'`.
- Mapeamento pra arquivo (todos já existem em `ui/public/sounds/`):
  - `reveal` → `revealing-card-table.mp3` (showdown genuíno revela cartas)
  - `showing_card` → `player-showing-card.mp3` (reveal voluntário, Task 3 acima)
  - `dealing` → `cards-on-table.mp3` (nova mão começa / board avança)
  - `half_pot` → `half-pot-chips.mp3` (uma aposta ≈ metade do pote atual)
  - `all_in` → `all-in-chips.mp3` (alguém fica all-in)
  - `bet` → alternar entre `basic-chips-1.mp3`/`basic-chips-2.mp3` (aposta/raise/call genérico, pra
    não soar repetitivo)

- [ ] **Step 1: `sound.ts`**

```ts
export type SoundName = 'reveal' | 'showing_card' | 'dealing' | 'half_pot' | 'all_in' | 'bet';

const FILES: Record<SoundName, string[]> = {
  reveal: ['/sounds/revealing-card-table.mp3'],
  showing_card: ['/sounds/player-showing-card.mp3'],
  dealing: ['/sounds/cards-on-table.mp3'],
  half_pot: ['/sounds/half-pot-chips.mp3'],
  all_in: ['/sounds/all-in-chips.mp3'],
  bet: ['/sounds/basic-chips-1.mp3', '/sounds/basic-chips-2.mp3']
};

export function playSound(name: SoundName) {
  const files = FILES[name];
  const file = files[Math.floor(Math.random() * files.length)];
  new Audio(file).play().catch(() => {});
}
```

- [ ] **Step 2: Disparar de dentro do `receive` callback**

Em `useTableRealtime.ts`, dentro do `receive` (evento, não render — seguro chamar `playSound` aqui),
ao lado da lógica já existente que monta `describeSnapshot`/`liveMessage`: comparar `previous`/`next`
e chamar `playSound(...)` nas transições relevantes (board cresceu → `dealing`; algum seat passou a
`all_in` → `all_in`; um bet/raise/call aumentou `contributed` → `bet`, e se o valor apostado for ≥
metade do pote anterior, `half_pot` no lugar; `stage` virou `complete` com `won_without_showdown ===
false` → `reveal`). O `ShowCardsCmd` bem-sucedido (resposta do próprio `rt.showCards()`) dispara
`showing_card` diretamente no callback de sucesso, não precisa de diffing.

> Nota pro implementador: esta é a parte mais subjetiva do plano — os limiares exatos (o que conta
> como "half pot") e a prioridade quando várias condições disparam no mesmo snapshot (ex.: board
> avançou E alguém ficou all-in no mesmo frame) ficam a critério de quem implementa; o importante é
> que cada `playSound` só dispare uma vez por transição real (comparar contra `previousSnapshot.current`
> exatamente como `describeSnapshot` já faz), nunca a cada broadcast.

- [ ] **Step 3: Lint, build, verificação manual**

```bash
cd ui && npx eslint src --max-warnings 0 && npx next build
```

Testar em duas abas reais (autoplay do navegador exige interação do usuário antes do primeiro som —
confirmar que isso não quebra a experiência, já que o `.catch` silencioso cobre esse caso).

- [ ] **Step 4: Commit**

```bash
git add ui/src/lib/sound.ts ui/src/lib/hooks/useTableRealtime.ts
git commit -m "feat(ui): wire existing sound assets to reveal/deal/bet/all-in events"
```

## Self-Review Notes

- **Reveal voluntário é ortogonal à correção do bug3**, não a desfaz: `revealAll` continua exigindo
  showdown genuíno; `VoluntarilyShown` é um opt-in por jogador, verificado separadamente.
- **`HandCategory` é por seat, não por mesa** — decisão deliberada: um showdown com 2+ mãos reveladas
  deve mostrar o tipo de CADA mão revelada, não só a vencedora.
- **Sons reaproveitam assets que já existem** no repo (`ui/public/sounds/*.mp3`, confirmado por
  listagem direta) — nenhum arquivo novo a conseguir/licenciar, só wiring.
- **Um ponto em aberto explicitamente flagado** (Task 4): falta decidir se `Snapshot` precisa de
  `WonWithoutShowdown` pra o client saber quando oferecer o botão "Mostrar cartas" sem heurística
  frágil — decisão do implementador, documentada como nota, não resolvida silenciosamente.
- **Fora de escopo:** Feature D (histórico/auditoria), conforme ordenação confirmada (bug3 → A → B →
  C → D).
