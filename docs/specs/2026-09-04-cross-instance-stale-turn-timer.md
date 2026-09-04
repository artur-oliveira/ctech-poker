# Incidente + Correção — timers de turno ficam obsoletos entre instâncias e distorcem o perimeter-timer/time bank

Data: 2026-09-04 · Escopo: `api/internal/table`

## 📌 Sintoma reportado

No `HandOutcomeBanner`/badge do jogo (`ui/src/components/table/HandOutcome.tsx`), o
perimeter-timer da contagem para a próxima mão contava ~2-3s e depois o jogo ficava parado
7-9s sem nenhum indicativo visual antes da próxima mão realmente começar. O mesmo padrão
aparecia no anel de turno normal e no time bank (`Seat.tsx`'s `SeatTurnTimer`/`SeatTimeBank`):
o cliente já renderizava "0s" no exato frame em que a decisão do jogador começava. O jogador
também relatou o time bank sendo consumido muito mais rápido que o normal, e o mesmo padrão ao
sair da mesa e sacar fichas (fluxo de `request_exit`).

## 📌 Investigação

1. **HAR do cliente** (`timer_errado.har`) decodificado (protobuf `ServerMessage`) mostrou que
   `next_hand_unix_ms` chegava ao cliente com só ~3,3s de folga em vez dos 12s configurados
   (`NextHandDelaySeconds`), mas a próxima mão de fato só começava ~12,17s depois — ou seja, o
   servidor estava contando certo, só o valor *anunciado* ao cliente é que nascia curto.
2. **Reprodução ao vivo** numa mesa real de produção (`01M1NZ8NKA95FMPT4S4TVXQ1GJ`), com logs
   puxados via `aws --profile ctech logs filter-log-events` no log group
   `/ctech-poker/prod/app`, confirmou a causa: a mesma instância EC2 roda dois processos
   (`app`/`app2`, streams de log distintos) e cada jogador desta mesa heads-up ficou conectado a
   um processo diferente. Achados nos logs:
   - `"table time bank consumed" hand=01M1NZD63X68TT6Y20ZMN4ZAF4 stage=complete` disparando 25s
     **depois** dessa mesma mão já ter sido enviada ao cliente como `complete`.
   - Para a mão seguinte, um log em `stage=complete` seguido, 2 segundos depois, de outro log
     **para a mesma `hand_id`** em `stage=flop` — logicamente impossível numa única execução
     sequencial: só é possível se duas instâncias tiverem visões divergentes e desatualizadas do
     mesmo hand.
   - Um `"table: actor stopped"` no meio da sessão (substituição de spot instance) coincidindo
     com o início da divergência.
3. **Causa raiz confirmada**: `internal/tablelease` é só de latência, nunca uma trava exclusiva
   fleet-wide (ver `api/CLAUDE.md`) — várias instâncias rodam `*Actor`s independentes para a
   mesma mesa por design, cada uma "confiando" no seu próprio cache (`a.trustCache`) até que algo
   force um reload. Os timers de turno/próxima-mão/runout
   (`api/internal/table/actor_timers.go`) são `time.AfterFunc` puramente em memória, sem
   contraparte persistida, armados no momento em que **aquela instância local** observou a
   transição. `handleTurnTimeout`, `handleNextHand` e `handleRunoutStep` chamavam
   `ensureLoaded(ctx, false)` — que, numa instância `trustCache=true`, pula o reload e reusa o
   cache local **por tempo indefinido**, até o próximo comando genuíno chegar naquela instância.
   Uma instância que fica quieta (o jogador dela não faz nada por um tempo) pode ter seu timer
   disparando minutos depois de a mão real já ter avançado — exatamente o padrão dos logs acima.

   A escrita condicionada por versão no DynamoDB (`Actor.commit`) já impede que esse disparo
   obsoleto **persista** uma ação incorreta na maioria dos casos (a tentativa da instância
   obsoleta esbarra em `ErrVersionConflict` e o próprio `handleTurnTimeout` já tinha um
   reload-e-recheque nesse branch) — mas o disparo em si já roda contra dados errados antes de
   chegar nesse ponto: loga (e, na janela entre o reload forçado e o commit seguinte, pode
   genuinamente competir e vencer) uma cobrança de time bank/decisão para um estágio que já não
   existe mais, e é exatamente esse mesmo mecanismo de commit atrasado que alimenta
   `next_hand_unix_ms`/`action_deadline_unix_ms` com um valor calculado num instante muito antes
   do broadcast que o cliente efetivamente recebe.

   Esse é o mesmo raciocínio que já existe em `actor_seating.go`'s `handleJoin` (força reload
   "porque outra instância pode ter commitado sem essa instância saber") — só nunca tinha sido
   aplicado aos handlers disparados por timer.

## 📌 Correção

`api/internal/table/actor_seating.go` (`handleTurnTimeout`) e `actor_timers.go`
(`handleNextHand`, `handleRunoutStep`) agora chamam `a.ensureLoaded(ctx, true)` em vez de
`ensureLoaded(ctx, false)` na entrada. Como esses três handlers só são alcançados por um
`time.AfterFunc` que pode ter sido armado um turno inteiro (+ time bank, + delay de próxima mão)
atrás, a janela de obsolescência do cache local — antes minutos, numa instância parada — cai
para a duração de uma leitura no DynamoDB. O guard existente
(`a.cached.CurrentPlayerIDForActor() != c.PlayerID`, e o equivalente em `handleNextHand`/
`handleRunoutStep` via `a.cached.Stage()`) passa a ser avaliado contra dado fresco, então uma
decisão/mão já resolvida em outra instância é corretamente ignorada em vez de recomputada contra
estado obsoleto.

**Fora do escopo desta correção, mesma classe de risco:** `handleKickTimeout` e
`handleAFKSweep` (`actor_presence.go`) e `handleExpireWinnerCards`
(`actor_player_actions.go`) também chamam `ensureLoaded(ctx, false)` a partir de um
`time.AfterFunc`. `handleKickTimeout` e `handleAFKSweep` já têm lógica própria para tolerar
cache cross-instance obsoleto (checam `LastActionAt` persistido, não só o marcador local de
desconexão) e o AFK sweep é um tick de 1 minuto — forçar reload nele custaria uma leitura extra
por mesa por minuto, sempre. Recomendado revisitar como item separado se aparecer evidência de
que essa mesma classe de bug afeta remoção/expiração de winner-cards, mas não fazia parte do
sintoma reportado.

## 📌 Sobre usar Valkey aqui

Cogitado (o padrão já existe em `internal/handhook`, `SET NX` por `(table_id, hand_id)` para
dedupe de hooks pós-mão). **Não é necessário para corrigir o dado persistido**: a escrita
condicionada por versão do DynamoDB já garante que só um commit "vence" por versão, então uma
instância obsoleta que tenta agir sobre uma decisão já resolvida acaba rejeitada e se
autocorrige pelo `ensureLoaded(ctx, true)` que o branch de conflito já fazia. O ganho real do
Valkey aqui seria puramente de eficiência/observabilidade — evitar a tentativa perdida e o log
confuso de uma instância que sabe, de antemão, que vai perder a corrida — não corrigir uma
falha de correção de dado. Dado que o reload forçado já fecha a janela de obsolescência de
minutos para um round-trip de leitura, não valeu a complexidade extra de uma claim distribuída
adicional para este caso. Reconsiderar se o volume de tentativas perdidas em produção
(`"table time bank consumed"` logs redundantes) se mostrar problemático por si só.

## 📌 Testes

`api/internal/table/crossinstance_integration_test.go` (`-tags integration`, requer DynamoDB
Local via `docker-compose.test.yml`) —
`TestStaleInstanceTurnTimeoutIgnoresATurnAnotherInstanceAlreadyAdvanced`: duas instâncias de
`Actor` compartilhando a mesma mesa; a instância B lê o estado uma vez (equivalente ao WS de um
jogador chegando) e nunca mais recebe comando algum; a instância A processa o fold real do
jogador que estava na vez, avançando o turno; o timeout obsoleto de B (chamado diretamente,
simulando o disparo tardio do `time.AfterFunc`) precisa ser um no-op total — versão e jogador da
vez inalterados no store.

Não foi possível rodar esse teste de integração neste ambiente (sem acesso ao socket do Docker
para subir o DynamoDB Local); `go build ./...`, `go vet ./...`, `go vet -tags integration
./...` e a suíte de testes unitários (`go test ./...`) do módulo `api` passam limpos. Rodar
`docker compose -f docker-compose.test.yml up -d && go test -tags integration ./internal/table/...`
antes de mergear.

## 📌 Limpeza de dados

Nenhuma migração. Mudança de comportamento pura em código (troca de `false` por `true` num
parâmetro já existente); nenhum schema ou dado persistido muda de forma.

## 📌 Atualização — o sintoma continuou após esse fix ir pra produção

O fix acima (`5a03b1f`/`094354a`/`7d1a411`) foi confirmado em produção
(`GET /v1.0/health` → `releaseId: "2609041218:7d1a411"`) e o sintoma **persistiu**: numa sessão
ao vivo com dois jogadores na mesma mesa, o anel de turno normal e o de time bank continuaram
aparecendo com `0s` desde o primeiro frame, e várias tentativas de agir a tempo (fold/all-in)
foram perdidas porque a decisão já estava resolvida no servidor antes do clique chegar. Isso
significa que a correção de `ensureLoaded` estava certa mas incompleta — o mecanismo raiz é outro.

### Causa raiz real

Cada instância EC2 já roda **dois processos Go atrás do nginx** (`APP_PORT`/`APP_PORT_ALT`,
`cdk/lib/constants.ts`, deploy zero-downtime — arquitetura antiga, de `1242bf1`). O nginx faz
round-robin entre eles, então o lease de uma mesa pode estar num processo enquanto o socket de um
jogador está no outro — cada processo mantém seu próprio `*Actor` independente.

O que de fato mudou o comportamento foi `666837c` ("refactor(api): bound table memory and split
actor", 2026-09-03 21:35 — o mesmo commit que dividiu `actor.go`), em
`api/internal/tablemanager/manager.go`. Antes:

```go
if hasLease {
    // arma eviction só na perda do lease
} else {
    go m.evictLeaseLessActorWhenIdle(runCtx, tableID, actor, cancel)
}
```

O ator dono do lease era **isento** de eviction por ociosidade. Depois:

```go
go m.evictActorWhenIdle(runCtx, tableID, actor, cancel)
```

roda incondicionalmente pra todo ator, lease ou não — qualquer ator com zero conexões WS locais
por 5 minutos contínuos é derrubado (issue #36, ver `docs/specs/2026-09-03-process-memory-bounds.md`,
uma correção de memória legítima). Como os dois jogadores podem cair em processos diferentes por
causa do round-robin, é comum o processo dono do lease não ter nenhum jogador conectado
localmente — antes isso não importava (o ator ficava vivo do mesmo jeito); agora, depois de 5 min,
esse ator é destruído. Quando ele é recriado, `ensureLoaded` lê `TurnDeadlineUnixMs`/
`NextHandDeadlineUnixMs` do DynamoDB, e `armTurnTimer`/`armNextHandTimer` **reusam esse valor de
propósito "mesmo que já tenha passado"** (pra resumir corretamente entre instâncias, comentário
já existente no código). Se a eviction acontecer no meio de um turno, o primeiro broadcast do
ator recém-recriado carrega um deadline já expirado — o "0s desde o primeiro frame".

O fix anterior (forçar reload nos handlers disparados por timer) não cobre esse caminho: ali o
problema era um timer **já armado** disparando tarde contra cache obsoleto; aqui o problema é um
ator **recém-recriado** reusando de propósito um deadline persistido que já venceu.

### Segunda correção

Em vez de reverter a eviction (ela corrige um vazamento de memória real, #36) ou tentar
sincronizar quando cada processo arma seu timer (exigiria um lock distribuído — Valkey resolveria
aqui, mas é mais complexidade do que o bug pede), a correção é bem mais direta: **nunca transmitir
ao cliente um deadline que já está no passado**, seja qual for a razão dele estar obsoleto.

`api/internal/table/actor_timers.go` ganha `Actor.deadlinesForBroadcast(currentPlayerID, stage)`,
chamado tanto por `broadcastAll` (`actor_views.go`) quanto por `handleSnapshot`
(`actor_loading.go`) — os dois únicos pontos que preenchiam `ActionDeadlineUnixMs`/
`ActionBaseDeadlineUnixMs`/`NextHandUnixMs`. Ele só devolve um valor quando o deadline
correspondente (`a.turnDeadline`/`a.nextHandDeadline`) ainda está estritamente no futuro; caso
contrário devolve zero (equivalente a "nenhum timer armado" pro cliente, que já trata `0` assim
em outros campos) e loga um `WARN` (`"table turn deadline already elapsed at broadcast"` /
`"table next-hand deadline already elapsed at broadcast"`) com o quanto o deadline já estava
vencido. O timeout real do servidor não muda — `armTurnTimer`/`armNextHandTimer` já agendam o
`time.AfterFunc` com delay `0` pra um deadline vencido, então a decisão é processada quase
imediatamente do mesmo jeito; a diferença é que o cliente nunca chega a renderizar um "é sua vez,
0s restantes" fantasma antes disso acontecer.

**Logs de diagnóstico temporários** (nível `INFO`, sem usuários reais na mesa agora — remover
depois de confirmar em produção): `armTurnTimer`/`armNextHandTimer` logam `"table turn timer
armed"`/`"table next hand timer armed"` toda vez que armam de verdade (não nos early-returns
idempotentes), com `resumed_from_persisted`, o deadline calculado e quanto tempo resta — dá pra
cruzar com o `WARN` de `deadlinesForBroadcast` pelo `table_id`/`hand_id`/`player_id` e ver
exatamente quando um deadline nasceu já vencido vs. venceu depois de armado mas antes do
broadcast.

Testes: `api/internal/table/deadlinesforbroadcast_test.go` (unitário, sem DynamoDB) —
`TestDeadlinesForBroadcastWithholdsAnAlreadyElapsedTurnDeadline` e
`...NextHandDeadline` cobrem: deadline futuro passa normalmente; deadline no passado é
retido (zero); deadline de outro jogador/mão nunca vaza. `go build ./...`, `go vet ./...`,
`go vet -tags integration ./...` e `go test ./...` do módulo `api` passam limpos.

**Fora do escopo:** não reverti `evictActorWhenIdle` (a eviction incondicional em si é
intencional e corrige um problema de memória real) nem investiguei se essa mesma eviction afeta
`handleKickTimeout`/`handleAFKSweep`/`handleExpireWinnerCards` da mesma forma — essa correção
ataca o sintoma (deadline obsoleto chegando ao cliente) na origem única onde ele é montado no
snapshot, então cobre qualquer causa de obsolescência, não só a eviction.
