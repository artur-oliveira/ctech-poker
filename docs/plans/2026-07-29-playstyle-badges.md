# Plano de Implementação — Badges de estilo públicos (opt-in)

Data: 2026-07-29 · Escopo: `api/`, `ui/` · Spec: `docs/specs/2026-07-28-player-avatars-and-next-features.md` (Feature 3)

## 📌 Contexto

`internal/pokerstats` já materializa tudo que o badge precisa. `Stats` (`stats.go:30-39`) carrega
`Hands`, `VPIPHands`, `PFRHands`, `ThreeBetHands`, `ThreeBetChances`, e as três taxas derivadas em
`calculateRates` (`stats.go:110-118`). A escrita já acontece por mão, idempotente, no hook
`onHandComplete` (`internal/app/app.go:251,274-276`), com guard item por `tableID#handID`
(`stats.go:53-63`). **Nenhum trabalho de agregação novo.**

O que existe hoje é só privado: `GET /v1.0/players/me/poker-stats` (`internal/api/v1/pokerstats.go:16`)
devolve `Stats` cru para o próprio dono, renderizado no `SelfHudDialog`.

## 📌 Nota de Arquitetura: a derivação de badge já existe — no cliente

`ui/src/components/lobby/SelfHudDialog.tsx:25-37` já tem `styleBadges(stats)`, com faixas próprias
(`vpip_rate <= .22` "Seletivo", `>= .38` "Explorador", `pfr_rate/vpip_rate >= .7` "Iniciativa",
`three_bet_chances >= 10 && three_bet_rate >= .1` "Contra-ataque", fallback "Equilibrado", `slice(0,3)`),
com piso de amostra de **30 mãos** em `:126`.

Isso muda o plano em dois pontos:

1. **Não é uma feature nova, é uma promoção.** A lógica sai do TS e vira dado em Go. O rótulo público
   e o rótulo do próprio HUD têm que ser **a mesma função** — dois conjuntos de faixas divergindo é
   pior que não ter badge, porque o jogador vê "Sólido" no seu HUD e o assento dele mostra outra coisa.
2. **`styleBadges` no TS tem que ser deletado**, não deixado ao lado. Se ficar, é a duplicata que
   divergirá no primeiro ajuste de calibração.

### O piso de amostra

A spec pede **200 mãos**; o cliente hoje usa 30. 200 é o número certo para badge *público*: 30 mãos de
VPIP é ruído, e ruído apresentado a um oponente como informação é pior que silêncio.

Manter os dois pisos, com nomes distintos e razões distintas:

- `MinHandsPublic = 200` — abaixo disso nenhum badge sai para terceiros.
- `MinHandsSelf = 30` — o próprio dono pode ver o seu badge provisório mais cedo, porque ele sabe que
  jogou 40 mãos. O `SelfHudDialog` já mostra o aviso de amostra baixa em `:127-129`.

### Onde o badge entra no assento

Já existe precedente exato para dado de perfil (não de motor) chegando no `SeatView`: o `Name`.
`SetNameCmd` (`internal/table/commands.go:169-173`) é preenchido pelo gateway a partir de
`player.Service`, nunca pelo cliente (comentário em `commands.go:164-168`), tratado em
`actor.go:432-452`, persistido no `Player` por `SetPlayerNameForActor`
(`internal/engine/hand/hand.go:288-299`) e lido no snapshot em `snapshot.go:221`. Dois sites de
dispatch: connect do WS (`tablews.go:363-372`) e sentar (`buyin/service.go:218-226`).

**Reusar esse caminho**, não abrir um segundo. A alternativa era o padrão do `playerNotes`
(fetch separado no cliente e merge por assento — `table/page.tsx:177-179` → `TableStage.tsx:50,88` →
`Seat.tsx:148-152`), mas notes são dado *privado do próprio viewer* (uma query serve todos os assentos).
Badge é dado público de terceiros: exigiria endpoint em lote, refetch a cada troca de assento e
invalidação. Um campo num comando que já é despachado nos dois momentos certos é menos código.

**Consequência aceita:** o badge é capturado no connect/sentar e não muda no meio da sessão. Correto
por construção — um badge com piso de 200 mãos não vira em 40 minutos de jogo.

⚠️ Se a Feature 1 (avatares) entrar antes, `SetNameCmd` já terá virado
`SetIdentityCmd{Name, AvatarURL}`; então este plano só acrescenta `PlaystyleBadge` ao struct que já
existe. Se entrar antes dos avatares, fazer o rename aqui e a Feature 1 herda.

---

## Fase 1 — Derivação em Go

### T1 — `internal/pokerstats/style.go`

Faixas como **tabela de dados**, no formato que o repo já usa (`internal/api/v1/stakes.go:16-31`,
`internal/achievements/catalog.go:37`), para poderem ser recalibradas sem mexer em lógica:

```go
// Badge é um rótulo derivado de Stats. Nunca expõe os números — número exato é
// vantagem competitiva assimétrica; o rótulo é o que um oponente atento já
// deduz observando.
type Badge struct {
    Key    string // "selective" | "explorer" | "initiative" | "counter" | "balanced"
    Label  string // pt-BR, exibido
    Reason string // tooltip; explica a faixa, não o valor do jogador
}

// MinHandsPublic é o piso de amostra para o badge sair para terceiros.
// Badge sobre 12 mãos é ruído apresentado como informação.
const MinHandsPublic = 200

// MinHandsSelf é o piso no HUD do próprio jogador, que sabe quantas mãos jogou.
const MinHandsSelf = 30

// Styles é a tabela de faixas, avaliada em ordem. Match cobre no máximo 3.
var styles = []struct {
    Badge Badge
    Match func(Stats) bool
}{ ... }

// StyleFor devolve até 3 badges, ou nil abaixo de minHands.
func StyleFor(s Stats, minHands int64) []Badge
```

Portar as faixas de `SelfHudDialog.tsx:25-37` verbatim nesta primeira passada — recalibrar e mudar o
dono da lógica no mesmo commit torna impossível saber qual dos dois causou uma diferença de rótulo.

`StyleFor` é pura, sem I/O: testável direto e reusável pelas três superfícies (HUD, showcase, assento).

### T2 — Opt-in no perfil (`internal/player/model.go`)

```go
PlaystylePublic bool `dynamodbav:"playstyle_public,omitempty" json:"playstyle_public"`
```

Campo novo em `model.go` (ao lado de `ShowcasePublic:24`). Default `false` — dado derivado do jogo de
alguém não vira público por omissão.

`UpdatePlayerRequest` (`internal/api/v1/player.go:24`) ganha `PlaystylePublic *bool`, seguindo o padrão
de ponteiro que o arquivo já documenta em `:18-19` ("ausente = não tocar"). Reusar o branch de
read-modify-write de `player.go:108-127`, não abrir outro.

Expor no `playerResponse` (`player.go:271`, onde `showcase_public` já sai) e no TS
(`ui/src/lib/api/player.ts:18`).

## Fase 2 — Superfícies

### T3 — Showcase público

`GET /v1.0/players/:playerId/showcase` (`internal/api/v1/player.go:202-249`) ganha, no `fiber.Map` de
`:243-248`:

```go
"playstyle": []fiber.Map{{"key": ..., "label": ..., "reason": ...}}, // omitido quando vazio
```

⚠️ **Esse endpoint é não-autenticado** — está registrado em `player.go:51`, *antes* do grupo com auth
que começa em `:52`. Então o badge fica legível por qualquer um na internet quando `playstyle_public`
está ligado. Isso é aceitável (é o que "público" significa) mas tem que ser dito na UI do toggle, não
implícito.

Dois gates, ambos obrigatórios: `PlaystylePublic` **e** `Hands >= MinHandsPublic`. `ShowcasePublic` já
gateia o endpoint inteiro (`player/service.go:99-107`), mas os dois opt-ins ficam separados: mostrar
troféus não é o mesmo consentimento que mostrar tendência de jogo.

O handler precisa de um leitor de stats novo — hoje só tem `players` e o índice de mãos. Injetar a
interface mínima (`Get(ctx, playerID) (Stats, error)`), no formato de `pokerstats.go:11-13`.

### T4 — Badge no assento

1. `internal/table/commands.go:169-173` — `SetNameCmd` ganha `PlaystyleBadge string` (a `Key` do badge
   primário, string vazia = sem badge). Só a chave: o rótulo em pt-BR e o tooltip são do cliente, e
   mandar texto no snapshot de toda mão é desperdício de frame.
2. `internal/engine/hand/hand.go:288-299` — `SetPlayerNameForActor` passa a `SetPlayerIdentityForActor`,
   mantendo o retorno `bool` de "mudou algo" que evita commit no-op.
3. `internal/engine/hand/snapshot.go:107-129` — `SeatView` ganha `PlaystyleBadge string`, preenchido no
   loop de `:218-228`.
4. `proto/poker.proto:13-43` — `Seat` ganha `optional string playstyle_badge = N;`. O próximo número
   livre hoje é **16**, mas a Feature 1 (avatares) reivindica o 16 na ordem sugerida da spec — então
   aqui é **17** se os avatares entrarem primeiro, 16 se este plano entrar antes. Confirmar o próximo
   livre no momento da implementação; nenhum `reserved` na message. Regerar Go + ts-proto. Conversão em
   `tablews.go:820-836`.
5. Os dois sites de dispatch resolvem o badge junto com o nome: `tablews.go:363-372` e
   `buyin/service.go:218-226`. Uma leitura a mais no `pokerstats.Store` por connect/sentar — é uma
   `GetItem` por `pk` único, não por mão.
6. Respeitar `PlaystylePublic` **no servidor**, no momento de montar o comando. Nunca mandar a chave e
   deixar o cliente esconder.

### T5 — UI

- **Deletar** `styleBadges` de `SelfHudDialog.tsx:25-37`. O dialog passa a ler os badges de
  `GET /v1.0/players/me/poker-stats`, que ganha o campo `playstyle` (com `MinHandsSelf`), e o gate de
  `:126` passa a ser "a API devolveu badges" em vez de `hands >= 30` calculado no cliente.
- Mapa `key → {label, reason}` em `ui/src/lib/playstyle.ts`, no formato de `@/lib/achievements`
  (`achievementLabel/Description`) que o `AchievementCard.tsx:6` já usa. É o único lugar onde texto
  pt-BR de badge existe.
- Pill visual: reusar `.poker-style-badges` (`globals.css:1888,1894`), que já é o formato de
  `SelfHudDialog.tsx:52-54`. No assento cabe **um** badge, ao lado do avatar — 3 pills num assento de
  poker é ruído.
- Consumidores: `Seat.tsx` (junto do avatar da Feature 1), `SelfHudDialog`, `app/profile/page.tsx`
  (showcase, `:36-79`).
- Toggle no `ProfileShowcaseDialog.tsx` — já é o dialog de opt-in, já tem `Switch` (`:53`) e a mutation
  (`:29`). Segundo switch, com o texto dizendo que o perfil público é acessível sem login.

## Fase 3 — Testes

Go, estilo do pacote (stdlib `testing`, sem testify — `pokerstats/stats_test.go:9,34`):

1. `StyleFor` abaixo de `MinHandsPublic` devolve `nil`. É o teste que impede badge de ruído.
2. Cada faixa da tabela produz o rótulo esperado; um caso por linha de `styles`.
3. Nunca mais de 3 badges.
4. Fallback "Equilibrado" quando nenhuma faixa casa.
5. Showcase com `playstyle_public: false` **omite** o campo mesmo com 5000 mãos.
6. Showcase com `playstyle_public: true` e 199 mãos omite; com 200 inclui.
7. `SeatView.PlaystyleBadge` vazio quando o opt-in está desligado — assertar no snapshot, não na UI.
8. **`GET /v1.0/hand-shares/:token` não expõe badge.** Mão compartilhada é anonimizada por alias;
   badge é sinal de identidade. Mesma regra que a Feature 1 aplica ao avatar, e pelo mesmo motivo.

UI (vitest, padrão de `lobbyComponents.test.tsx:41-85` com `vi.hoisted` mockando react-query):

9. `SelfHudDialog` sem badges na resposta não renderiza a seção.
10. Assento sem badge renderiza idêntico ao de hoje (snapshot).

## 📊 Resultado esperado

| Antes                                        | Depois                                        |
|----------------------------------------------|-----------------------------------------------|
| Faixas de badge em TS, só no HUD do dono     | Faixas em Go, tabela de dados, três superfícies |
| Piso de amostra 30, calculado no cliente     | 200 para público, 30 para o próprio, no servidor |
| Estilo do oponente só para quem tem HUD externo | Simétrico, opt-in, rótulo sem números       |
| Um opt-in (`showcase_public`)                | Dois consentimentos separados                 |

## 🔮 Fora deste plano

- **Recalibrar as faixas.** Portadas verbatim de propósito. Calibrar depois, com dados de produção, e
  num commit que só mexa na tabela.
- **Badge por posição ou por street** (VPIP de BTN vs UTG). `pokerstats` não segmenta hoje; segmentar
  multiplica a escrita por mão.
- **Histórico de badge / evolução no tempo.** `Stats` é acumulador desde sempre, sem janela. Badge com
  janela móvel exigiria outro modelo de dados.
- **Expor os números brutos publicamente.** Decisão explícita: nunca. O rótulo é derivável por
  observação, o número exato não.
