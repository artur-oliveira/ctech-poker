# Plano de Implementação — Clubes / ligas privadas

Data: 2026-07-29 · Escopo: `api/`, `ui/`, `cdk/` · Spec: `docs/specs/2026-07-28-player-avatars-and-next-features.md` (Feature 5)

## 📌 Contexto

Grupo persistente de jogadores com ranking próprio e temporadas. É o maior dos cinco features da spec, e
o único que levanta pergunta regulatória nova.

Base existente: `roomstore` já tem sala privada com `share_code`, e `leaderboard` já agrega por mão.

## 📌 Nota de Arquitetura: sandbox primeiro, e não é preciosismo

Clube de dinheiro real é **organização de jogo entre pessoas determinadas** — exatamente a área cinzenta
de `OVERVIEW.md § 11`, e uma figura jurídica diferente da que o parecer de 2026-07-28 cobre. Aquele
parecer autoriza uma **taxa de reserva de mesa** num serviço aberto; um clube fechado com ranking e
temporada tem cara de outra coisa.

Portanto: `POST /v1.0/clubs` rejeita `currency_mode: "real"` **incondicionalmente**, não atrás de
`RealMoneyEnabled`. O gate de `rooms.go:65-67` é uma flag de config porque real-money de mesa aberta já
tem parecer; aqui não há parecer nenhum, então não há flag para ligar. Quando houver, vira flag.

### Ranking de clube não é um GSI a mais na tabela global

`leaderboard` tem `pk = playerID`, `sk = "stats"`, e três GSIs cuja partição é a **string literal
`"all"`** (`store.go:36-40`) — `gsi_hands_won`, `gsi_hands_played`, `gsi_win_rate` (CDK
`dynamodb-stack.ts:103-121`). Isso é um índice de partição única: funciona porque há um ranking só.

Escopo por clube quer **par PK/SK novo**, não um quarto GSI: a partição passa a ser
`club#<id>#season#<n>`. Um GSI a mais na tabela global manteria a partição `"all"` e não conseguiria
particionar por clube.

### O ranking de clube herda o problema do sort key derivado — e desvia dele

`Store.materializeWinRate` (`store.go:74-104`) existe porque `win_rate_score` é *derivado* de
`hands_played`/`hands_won` e um GSI não indexa expressão: ele materializa o valor num loop condicional de
**5 tentativas**. É código correto e chato de manter.

O ranking de clube v1 ordena **só por valores acumulados diretamente** (`hands_won`, fichas líquidas) —
um `ADD` de `UpdateItem`, sem loop de materialização. Win-rate de clube fica fora até alguém pedir. Um
loop de retry condicional por mão por jogador por clube é custo de escrita que não se paga em ranking de
amigos.

Nota herdada de lá: `achievement_points` é deliberadamente não-rankeável (comentário
`leaderboard/service.go:72-74`). Manter a mesma restrição no clube.

### Uma tabela, não duas

A spec propõe `poker_clubs` + `poker_club_members`. **Uma tabela só**, com o `pk`/`sk` que o factory de
CDK já impõe (`dynamodb-stack.ts:31-48`, `pk`/`sk` sempre STRING):

```
pk = club#<clubID>
sk = meta                  -> metadados (nome, dono, invite_code, created_at)
sk = member#<playerID>     -> filiação (joined_at, role)
sk = rank#<seasonID>#<playerID> -> acumulado da temporada
```

Uma `Query` no `pk` traz clube + membros. Temporada nova é prefixo novo de `sk`: **sem migração**, e a
temporada anterior continua consultável. Duas tabelas custariam duas leituras para renderizar uma tela.

Dois GSIs esparsos, exatamente no padrão que `roomstore` já usa (atributos de índice injetados no
encode em `dynamo.go:34-35`, esparsidade por `publicIndexValue:51-56`):

- `gsi_member` — partição `playerID`, populado **só** nos itens `member#`. Responde "de quais clubes
  esse jogador é membro".
- `gsi_invite_code` — partição do código, **só** no item `meta`. Espelha `gsi_share_code`
  (`dynamodb-stack.ts:86-90`).
- `gsi_club_rank` — partição `club#<id>#season#<n>`, ordenação por `hands_won`. Único índice com sort
  key numérica.

---

## Fase 1 — Fundação

### T1 — Tabela (`cdk/lib/dynamodb-stack.ts`)

`poker_clubs` via o factory de `:31-48` (`table('poker_clubs', true)` — com sort key, sem TTL, sem
stream), mais os três GSIs no formato verbatim de `poker_rooms` (`:80-90`). Entrada no union `TableName`
de `:10-14`.

Tabela **16** do serviço. Adicionar ao array de ARNs do role da instância em `api-stack.ts:118-122`
(hoje 14 entradas — nota: `poker_pending_cashouts` deliberadamente não está lá, porque só a Lambda de
reconcile a usa; `poker_clubs` **precisa** estar).

O statement de `:123-135` já cobre `resources: [...tableArns, ...tableArns.map(arn => \`${arn}/index/*\`)]`
em `:134`, então os GSIs vêm de graça. Threading em `bin/poker.ts` no formato de `:84-97`.

Teste de CDK no formato de `cdk/test/dynamodb-stack.test.ts` (a assertion de `gsi_share_code` em `:105` é
o modelo).

### T2 — Pacote `internal/clubs`

Formato de `internal/playernotes` (o store pequeno mais recente): `store.go` com const de tabela (`:15`),
erros sentinela (`:19-23`), modelo, `type Store struct{ base dynamo.Base }` (`:33`),
`NewStore(db *dynamodb.Client, env string) *Store` (`:35-37`), e funções puras testáveis sem cliente
(`Normalize:39-54` é o precedente — `store_test.go:9-37` testa só ela).

```go
type Club struct {
    ID, Name, OwnerID, InviteCode string
    SeasonID   int    // temporada corrente; incrementar é a única operação de rollover
    MemberCount int
    CreatedAt  string
}

type Member struct{ PlayerID, Role, JoinedAt string } // role: "owner" | "member"

func (s *Store) Create(ctx, c Club) error
func (s *Store) Get(ctx, clubID string) (*Club, error)
func (s *Store) GetByInviteCode(ctx, code string) (*Club, error)   // gsi_invite_code
func (s *Store) ListMembers(ctx, clubID string, limit int, startKey ...) ([]Member, ..., error)
func (s *Store) ListForPlayer(ctx, playerID string) ([]Club, error) // gsi_member
func (s *Store) AddMember(ctx, clubID, playerID, role string) error // PutItem condicional
func (s *Store) RemoveMember(ctx, clubID, playerID string) error
func (s *Store) IncrementRank(ctx, clubID string, seasonID int, playerID string, handsWon, netChips int64) error
```

`AddMember` com `PutItem` condicional em `attribute_not_exists(sk)`: a condição é o que impede
`MemberCount` de contar duas vezes numa corrida de duplo-clique no convite.

Wiring Fx, os cinco pontos que `playernotes` toca:
`newClubStore` em `app.go` (formato de `:194-196`) → `fx.Provide` (`app.go:62` é a linha vizinha) →
param de `registerRoutes` (`app.go:443`) → `v1.Register` (`app.go:447`) → assinatura em `router.go:49` →
`RegisterClubs` em `router.go:74`.

### T3 — Código de convite

**Reusar, não reinventar.** `newShareCode()` (`rooms.go:300`) já é 6 bytes de `crypto/rand` em 12 hex
maiúsculos, com a justificativa em comentário em `:298-299`. E a comparação de tempo constante já existe
em `privateRoomAccessAllowed` (`rooms.go:242-248`, `subtle.ConstantTimeCompare` em `:247`, com bypass do
criador em `:243`).

Extrair as duas para um lugar compartilhado — hoje `newShareCode` está privado em `rooms.go`. Sala e
clube gerando código por caminhos diferentes é como um dos dois acaba com `math/rand`.

⚠️ `sanitizeRoom` (`rooms.go:231-237`) tira o `ShareCode` de quem não é o criador, e é chamado em
`:191` e `:205`. **O clube precisa do equivalente**: `GET /v1.0/clubs/:id` não pode devolver
`invite_code` para não-membro. Código de convite vazando é o clube deixando de ser privado.

## Fase 2 — API

### T4 — Rotas

Formato de `playernotes.go:13-30`: interface estreita para os handlers (`:13-16`), struct de handler
(`:18`), DTO (`:20-23`), `RegisterClubs(router fiber.Router, auth fiber.Handler, store clubStore)` com
`g := router.Group("/clubs", auth)`. Identidade por `c.Locals(localsUserID)` (`auth.go:11,29`).
Paginação por `helpers.go` (`decodeCursor:31`, `buildNextCursor:52`, `limitParam:67`, `sendPage:79`).

| Rota | Nota |
|---|---|
| `POST /v1.0/clubs` | cria; criador vira `owner`. Rate-limited (novo limiter em `router.go:68-70`) |
| `GET /v1.0/clubs` | clubes do jogador, via `gsi_member` |
| `GET /v1.0/clubs/:id` | metadados + membros paginados; `invite_code` só para membro |
| `POST /v1.0/clubs/join` | `{invite_code}`; compare de tempo constante |
| `DELETE /v1.0/clubs/:id/members/me` | sair |
| `DELETE /v1.0/clubs/:id/members/:playerId` | só `owner` |
| `GET /v1.0/clubs/:id/ranking?season=N` | `gsi_club_rank`; `season` default = corrente |
| `POST /v1.0/clubs/:id/seasons` | só `owner`; incrementa `SeasonID` |

Limites duros, na criação e no join: máximo de clubes por jogador e de membros por clube. Sem eles,
`gsi_member` e a `Query` de membros crescem sem teto — e é mais fácil escolher um número agora que
migrar depois.

### T5 — Sala de clube

`roomstore.Room` (`room.go:6-33`) ganha `ClubID string` com `dynamodbav:"club_id,omitempty"`.
`CreateRoomRequest` (`roomdto.go:5-16`) ganha `ClubID string`.

Validação em `createRoom` (`rooms.go:50-156`), no bloco de `:52-99`:

- `club_id` presente → exige que o criador seja membro (uma `GetItem` em `club#<id>` / `member#<uid>`).
- `club_id` presente → **força `visibility: "private"`**, e a sala herda os membros: não precisa de
  `share_code` circulando. `privateRoomAccessAllowed` (`:242-248`) ganha um terceiro ramo — membro do
  clube entra sem código.
- `club_id` presente + `currency_mode: "real"` → **400 incondicional**. Ver a nota de arquitetura.

`sanitizeRoom` (`:231-237`) mantém `club_id` (não é segredo) e continua tirando `share_code`.

### T6 — Agregação de ranking

O hook `onHandComplete` (`app.go:251`) é **síncrono** e já faz três coisas: `pokerStatsStore.RecordHand`
(`:276`), `leaderboardSvc.RecordUnlocks` (`:267`) e `leaderboardSvc.RecordHand` (`:270`).

Acrescentar o clube significa: ler a sala para descobrir `ClubID`, e escrever `IncrementRank` por
participante. **Custo por mão: uma leitura + N escritas.** Duas mitigações, ambas necessárias:

1. **Curto-circuito antes de qualquer I/O.** `ClubID` vazio (o caso comum, e o único caso hoje) não faz
   leitura nenhuma. O `ClubID` já vem no `roomstore.Room` que o ator carregou; não abrir uma segunda
   consulta.
2. **Falha nunca é fatal.** É o que `pokerStatsStore.RecordHand` já faz — erro só logado
   (`app.go:276`). Ranking de clube atrasado é aceitável; mão travada por escrita de ranking não é.

`IncrementRank` é um `UpdateItem` com `ADD`, no formato de `IncrementStats` (`leaderboard/store.go:33`) —
**sem** o loop de `materializeWinRate` (`:74-104`), porque nenhuma métrica de v1 é derivada.

## Fase 3 — UI

### T7 — Telas

Rotas novas em `ui/src/app/` (13 hoje, nenhuma `clubs/`): `/clubs` (lista + criar + entrar por código) e
`/clubs/[id]` (membros, ranking, criar mesa do clube).

`app/lobby/page.tsx:76-85` é a nav a estender (hoje Guia/Ranking/Conquistas/Mãos + `ProfileMenu`).

Ranking: clonar `app/leaderboard/page.tsx` (split de top-3 em `:16-18`, card do viewer em `:40-45`).

### T8 — Entrar por código: não existe hoje

O único fluxo de convite é **por link**: `table/page.tsx:142` lê `params.get('invite')`, passa para o hook
(`:181`) e para o buy-in (`:315`); `InviteDialog.tsx` (57 linhas) copia/compartilha a URL
(`:18-26`, `:28-38`). **Não há formulário de digitar código em nenhum lugar** — e `ui/src/lib/api/rooms.ts`
nem tem função para `GET /v1.0/rooms/code/:code`, apesar de o endpoint existir (`rooms.go:42`).

Então o formulário de "entrar no clube por código" é **novo**, não clonado. Construir simples (input +
submit, erro de código inválido) e reusar em ambos: expor também `getRoomByShareCode` em `rooms.ts`,
fechando essa lacuna de passagem.

`CreateRoomDialog.tsx` (schema zod em `:26-30`, submit em `:68-90`) ganha o `club_id` quando aberto de
dentro de `/clubs/[id]`.

## Fase 4 — Testes

Padrão do repo: dois níveis — puro/fake em teste normal, DynamoDB Local atrás de `//go:build integration`
(cliente em `localhost:8555` no formato de `roomstore/dynamo_test.go:17-29`, tabela criada pelo próprio
teste em `mustCreateTestTable:34-72`, `docker-compose.test.yml`). Fake de store por interface estreita no
formato de `leaderboard/service_test.go:12-35`. Sem testify.
⚠️ `api/Makefile:17-18` não tem target com a tag `integration` — os testes de integração hoje só rodam à
mão. Vale um target junto.

1. **`AddMember` concorrente adiciona uma vez.** Integração; a condicional é o mutex e mutex sem teste
   contra o banco é fé.
2. **Código de convite compara em tempo constante** e código errado não distingue "clube não existe" de
   "código errado".
3. **`invite_code` não sai para não-membro** em `GET /clubs/:id`. É o teste de vazamento.
4. **`gsi_member` devolve só clubes do jogador** (índice esparso não vaza item `meta` nem `rank#`).
5. **Sala com `club_id` é forçada a privada** e membro entra **sem** `share_code`.
6. **Sala de clube com `currency_mode: "real"` é rejeitada** mesmo com `RealMoneyEnabled` ligado. É o
   teste que trava a decisão regulatória em código, não em comentário.
7. **Não-membro não cria sala do clube.**
8. **Ranking soma por temporada**; virar temporada não apaga nem mistura a anterior.
9. **`ClubID` vazio não faz I/O de clube** no `onHandComplete`. Assertar zero chamadas no fake — é a
   regressão de custo por mão.
10. **Falha de `IncrementRank` não falha a mão.**
11. **Remover membro**: só `owner` remove terceiro; qualquer um sai de si mesmo; `owner` não sai sem
    transferir.
12. **Limites duros** de clubes por jogador e membros por clube.

UI (vitest): lista de clubes vazia tem estado próprio; código inválido mostra erro sem navegar; criar mesa
de dentro do clube manda `club_id`.

## 📊 Resultado esperado

| Antes                                       | Depois                                                 |
|---------------------------------------------|--------------------------------------------------------|
| Sala privada por link, sem grupo persistente | Clube com membros, convite por código, temporadas      |
| Um ranking global (partição `"all"`)        | Ranking por `club#<id>#season#<n>`                     |
| Convite só por link                         | Formulário de código, reusado por sala e clube         |
| 15 tabelas DynamoDB                         | 16                                                     |

## 🔮 Fora deste plano

- **Clube com dinheiro real.** Bloqueado por decisão, no código, não em comentário. Precisa de parecer
  próprio: organização de jogo entre pessoas determinadas é figura diferente da taxa de reserva de mesa.
- **Win-rate no ranking de clube.** Exigiria o loop de materialização de `store.go:74-104` por mão por
  clube.
- **Torneios de clube / temporada com premiação.** Premiação reabre a pergunta regulatória inteira.
- **Chat ou mural de clube.** Superfície de moderação nova; `Chat` de mesa já existe e não cobre isso.
- **Papéis além de `owner`/`member`.** Admin/tesoureiro quando houver demanda.
- **Rollover automático de temporada.** Manual pelo dono primeiro; agendar depois exige Scheduler e DLQ,
  que hoje faltam nos dois targets existentes (`docs/README.md`).
