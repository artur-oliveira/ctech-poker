# Plano de Implementação — Reserva de Mesa como Entitlement (`ctech-poker`)

Data: 2026-07-28 · Escopo: `api/`, `cdk/` · Fora de escopo: integração ASAAS (é do `ctech-wallet`)

## 📌 Contexto e base legal

A receita do modo dinheiro real é uma **taxa fixa de reserva de mesa**, nunca rake percentual. A base
jurídica (parecer confirmado 2026-07-28) é que a taxa paga um *serviço de reserva*, cujo custo é
infraestrutura — portanto o lucro **não pode** derivar da perda de outro jogador, e a taxa **não pode**
escalar com o blind nem com aposta.

O catálogo em `internal/api/v1/stakes.go` já respeita isso: 4 tiers (Micro/Low/Mid/High =
R$1/2/4/8), chato dentro do tier, lookup armazenado e nunca derivado no momento da cobrança — os
blinds variam 1000× enquanto a taxa varia 8×. **Nada disso muda neste plano.**

O que está errado é *quando* a taxa é cobrada.

### Problema 1 — a taxa é cobrada em todo rebuy

`internal/buyin/service.go:243` cobra `room.EntryFeeCents` dentro de `BuyIn`, e `BuyIn` **é também o
caminho de rebuy**: `:168-170` deixa passar deliberadamente o jogador sentado com `Stack <= 0`
("busted, still occupying the seat"), e `:229-233` incrementa `open.BuyinAmount` numa sessão já aberta.
O `feeKey` em `:244` carrega um nonce novo por chamada, então nada dedupa.

Resultado: quem quebra 5× no Micro paga R$5 numa sentada só. **A receita da taxa fica proporcional à
frequência com que o jogador perde a stack**, que é exatamente o que a base legal exige que não
aconteça. Sair da mesa e voltar tem o mesmo efeito.

### Problema 2 — a taxa é debitada depois de sentar

`:243-256` roda **após** o `JoinCmd` de `:196`. Se o `DebitReal` falha, o jogador já está sentado e
jogando, o débito vai para `poker_pending_cashouts` (`Kind: fee_debit`) e o handler ainda retorna erro
para um cliente que de fato sentou. Dá para jogar sem ter pago, e o cliente recebe um sinal falso.

## 📌 Nota de Arquitetura: por mesa, não por tier

A reserva é escopada à **mesa**, não ao tier. Duas razões:

1. O parecer cobre um produto nomeado — "serviço de reserva de mesa". Um passe por tier é outro
   produto ("acesso ao tier") e exigiria voltar ao advogado. Não redesenhar produto por baixo de um
   parecer.
2. Blind diferente tem preço diferente; um passe por tier obrigaria a decidir qual preço ele cobre.
   Por mesa isso já está resolvido pelo lookup no catálogo.

O tier entra apenas como critério de **elegibilidade de rebind** (abaixo) e como origem do preço.

### O rebind

Escopar à mesa cria dois problemas reais, ambos com a mesma solução:

- **A mesa morre debaixo do jogador.** `cmd/tablecleanup` arquiva mesa parada há 15 min. Paga a mesa A
  do Micro, todos saem, a mesa é limpa, entra na mesa B do Micro → cobrado de novo. Isso reintroduz a
  aparência de cobrança por sentada.
- **A mesa lota enquanto ele está fora.** Pagou, fez cash-out, voltou, não tem assento. Reserva sem
  valor e ticket de suporte.

**Solução:** a reserva não morre quando a mesa paga fica indisponível — ela passa a apontar para outra
mesa do mesmo tier, dentro da janela restante. "Você reservou uma mesa, nós a fechamos, aqui está uma
equivalente" é justo e trivial de defender. Um mecanismo, dois problemas.

A reserva dá **acesso à mesa, não assento garantido** (garantir assento exigiria segurar cadeira vazia
após cash-out, bloqueando jogador pagante). Isso precisa estar escrito nos termos de uso.

### A janela

**3 horas, absolutas, gravadas na compra.** Escolhido para equilibrar o custo provável de
leituras/escritas e uso de recursos contra o tempo pago. Nunca deslizante — janela que renova a cada
uso é poker grátis para sempre.

---

## Fase 1 — Entitlement

### T1 — Tabela `poker_table_entitlements` (`cdk/lib/dynamodb-stack.ts`)

```
pk = playerID
sk = ent#<tableID>     // a mesa ORIGINALMENTE paga; imutável, é a chave de idempotência
attrs:
  bound_table_id  (S)  // mutável: a mesa a que a reserva aponta agora
  tier            (S)  // "micro" | "low" | "mid" | "high"
  fee_cents       (N)  // o que foi efetivamente cobrado, para auditoria
  expires_at      (N)  // unix seconds, absoluto
  created_at      (N)
  ttl             (N)  // expires_at + folga, para o item sumir sozinho
```

`sk` guardar a mesa original e `bound_table_id` ser o campo mutável evita mutar chave no rebind: vira
`UpdateItem`, não delete+put. Mesmas convenções das outras 15 tabelas (`TableV2`, `pk` string,
on-demand com teto 1000, PITR só em prod, prefixo `<env>_`).

Adicionar a tabela ao array de ARNs da policy DynamoDB do role da instância em `cdk/lib/api-stack.ts`
(hoje 14 ARNs) — `GetItem`, `Query`, `PutItem`, `UpdateItem`.

Sem GSI: a consulta é sempre `Query` no `pk` de um jogador, que tem um punhado de itens. Filtrar em
memória é mais barato que manter índice.

### T2 — Pacote `internal/entitlement`

Espelhar o formato dos stores existentes (`internal/playernotes`, `internal/dailyreward`): um `Store`
sobre `dynamo`, sem lógica de negócio de cobrança dentro.

```go
type Entitlement struct {
    PlayerID, OriginTableID, BoundTableID, Tier string
    FeeCents  int64
    ExpiresAt time.Time
}

// Claim grava a reserva com PutItem condicional em attribute_not_exists(sk).
// O erro condicional é o mutex: dois BuyIn concorrentes do mesmo jogador na
// mesma mesa não podem cobrar duas vezes. Retorna ErrAlreadyClaimed.
func (s *Store) Claim(ctx context.Context, e Entitlement) error

// ActiveFor devolve as reservas não expiradas do jogador (Query no pk, filtro
// por expires_at em memória).
func (s *Store) ActiveFor(ctx context.Context, playerID string) ([]Entitlement, error)

// Rebind aponta uma reserva existente para outra mesa. UpdateItem condicional
// em attribute_exists(sk) AND expires_at > :now, para não ressuscitar reserva
// expirada numa corrida.
func (s *Store) Rebind(ctx context.Context, playerID, originTableID, newTableID string) error
```

`ExpiresAt` é calculado **pelo chamador** na compra (`now + entitlement.Window`), com a janela como
constante nomeada única:

```go
// Window é a validade da reserva de mesa. Absoluta, contada da compra: uma
// janela deslizante tornaria a taxa cobrável uma única vez por jogador.
const Window = 3 * time.Hour
```

### T3 — Tier no catálogo de stakes (`internal/api/v1/stakes.go`)

`publicStake` hoje só tem `FeeCents`; o tier existe apenas como comentário. O rebind precisa comparar
tiers, então o tier passa a ser dado:

```go
type publicStake struct {
    SmallBlind, BigBlind int64
    Tier                 string // "micro" | "low" | "mid" | "high"; vazio em sandbox
    FeeCents             int64
}
```

Preencher os 10 stakes reais existentes com o tier que os comentários já indicam. Expor `Tier` no JSON
do catálogo (o frontend precisa dele para explicar a reserva ao jogador). `realStakeFeeCents` ganha uma
irmã `realStakeTier`, ou passa a devolver `(feeCents, tier, ok)` — preferir a segunda, um lookup só.

Guardar `Tier` também em `roomstore.Room` na criação (`internal/api/v1/rooms.go:110-124`, ao lado de
`EntryFeeCents`), para não re-derivar do blind depois. **Constraint global do repo:** a taxa e o tier
nunca são derivados do blind no momento da cobrança, sempre lidos do que foi gravado.

## Fase 2 — Cobrança

### T4 — Mover a cobrança para antes de sentar e passar pelo entitlement

Reescrever o bloco `internal/buyin/service.go:243-256`, movendo-o para **antes** do
`actor.Dispatch(table.JoinCmd{...})` de `:196`, e antes do hold/debit da stack.

Ordem nova dentro de `BuyIn`, quando `room.CurrencyMode == "real" && room.EntryFeeCents > 0`:

```
1. ActiveFor(playerID)
2. existe reserva com bound_table_id == roomID          -> segue, grátis
3. existe reserva do mesmo tier cuja mesa está
   indisponível (arquivada ou lotada)                   -> Rebind, segue, grátis
4. nada                                                 -> Claim condicional; se OK, DebitReal;
                                                           se ErrAlreadyClaimed, tratar como (2)
5. hold/debit da stack (código atual)
6. JoinCmd (código atual)
```

Rebuy na mesma mesa cai em (2), então o Problema 1 morre sem regra extra. Sair e voltar dentro das 3h
também.

**Ordem de `Claim` e `DebitReal`:** gravar o entitlement **antes** de debitar, e se o débito falhar,
apagar o entitlement. O inverso (debitar e então gravar) deixa o jogador pagando sem reserva se a
escrita falhar — sempre falhar na direção que não cobra sem entregar. Se o *delete* de compensação
falhar, o jogador ficou com reserva grátis: registrar `ALARM:` e seguir. Perder R$1 de receita é
preferível a cobrar sem entregar, e é o mesmo critério que `:205-213` já aplica ao refund da stack.

Se o `DebitReal` falhar de forma retentável, manter o comportamento atual de gravar em
`poker_pending_cashouts` com `Kind: fee_debit` — mas agora o jogador **não sentou**, então o retry não
está perseguindo alguém que já está jogando.

### T5 — "Mesa indisponível" para efeito de rebind

O passo (3) precisa decidir se a mesa originalmente paga está indisponível. Duas condições, ambas já
consultáveis:

- **Arquivada ou inexistente**: `roomstore.Get` devolve nil, ou `tablestore` marca `archived`
  (`cmd/tablecleanup` já usa `MarkArchived`).
- **Lotada**: assentos ocupados `>= room.MaxSeats`. `buyin` já obtém snapshot via `s.isSeated`; extrair
  daí uma contagem em vez de abrir um segundo caminho.

Se a mesa original ainda está viva e com vaga, **não** há rebind: o jogador tem reserva válida lá e
está escolhendo outra mesa, que é uma segunda reserva. Multi-mesa (2–4 mesas simultâneas) fica coerente
de graça — cada mesa é uma reserva.

### T6 — Expor a reserva ao cliente

`GET /v1.0/rooms/:id/seated` já é o pré-check de assento do cliente. Acrescentar ao corpo:

```json
{ "seated": false, "stack": 0,
  "entry_fee_cents": 100,
  "fee_due": true,
  "entitlement_expires_at": null }
```

`fee_due: false` quando existe reserva válida (própria mesa ou rebindável), para o cliente conseguir
dizer "sua reserva vale até 14:32" em vez de surpreender com cobrança no buy-in. Cobrança silenciosa
num fluxo de dinheiro real é ticket de suporte garantido.

## Fase 3 — Testes

Testes de unidade em `internal/entitlement` e `internal/buyin` (o pacote já tem fake de `walletMover`,
estender em vez de criar outro):

1. **Rebuy não cobra duas vezes.** Buy-in, bust (`Stack <= 0`), rebuy → um `DebitReal` só. É o
   Problema 1; este teste é o que impede a regressão.
2. **Sair e voltar dentro da janela não cobra.** Cash-out, buy-in de novo → um `DebitReal`.
3. **Depois da janela cobra.** Relógio injetado avançando > 3h → segundo `DebitReal`.
4. **Concorrência não cobra duas vezes.** Dois `BuyIn` paralelos do mesmo jogador na mesma mesa → um
   `DebitReal`; o perdedor da corrida vê `ErrAlreadyClaimed` e segue como reserva válida.
5. **Concorrência não emite grátis.** Dois `BuyIn` paralelos sem reserva prévia → exatamente um
   entitlement gravado.
6. **Rebind por mesa arquivada** não cobra; **rebind por mesa lotada** não cobra; **mesa viva com
   vaga** cobra (segunda reserva).
7. **Rebind só dentro do mesmo tier.** Reserva de Micro não libera mesa de Low.
8. **Janela é absoluta.** Vários buy-ins dentro da janela não empurram `expires_at`.
9. **Falha no débito não senta o jogador** e não deixa entitlement órfão.
10. **Sandbox não é afetado.** Nenhuma leitura ou escrita de entitlement em `currency_mode: sandbox`.

Integração (`tests/integration`, DynamoDB Local) para o `Claim` condicional e o `Rebind` condicional —
a condição é o mutex, e mutex sem teste contra o banco real é fé.

## 📊 Resultado esperado

| Antes                                         | Depois                                    |
|-----------------------------------------------|-------------------------------------------|
| Taxa por buy-in, incluindo cada rebuy         | Taxa por reserva de mesa, 3h              |
| Receita proporcional à frequência de bust     | Receita independente de resultado de jogo |
| Reentrada após cash-out cobra de novo         | Grátis dentro da janela                   |
| Mesa arquivada força nova cobrança            | Reserva faz rebind para mesa equivalente  |
| Cobrança após sentar; dá para jogar sem pagar | Cobrança antes de sentar                  |
| Cobrança silenciosa no buy-in                 | `fee_due` no pré-check de assento         |

## 🔮 Fora deste plano

- **Integração ASAAS / subcontas / hold no ledger** — escopo do `ctech-wallet`. O poker fala com
  `walletclient.DebitReal` e não deve saber que existe ASAAS. Invariante que o wallet precisa gravar
  explicitamente: *todo* movimento de dinheiro passa pela API; se o titular ganhar acesso direto à
  conta, o hold em ledger vira decorativo.
- **Gate de KYC no buy-in real** — `jwtverify.Claims` já carrega `KYCLevel`; exigir `verified` é uma
  linha, mas é decisão de produto e não deste plano.
- **Cobrança do criador na criação da sala** — hoje o criador só paga se sentar. Cobrar na criação
  casaria melhor com "reserva", mas abre política de reembolso quando ninguém aparece. Mantido como
  está.
- **Reembolso proporcional de reserva não usada.** Fora de escopo; se entrar, é decisão jurídica antes
  de ser técnica.
