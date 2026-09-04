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

## 📌 Segunda atualização — jogando ao vivo de novo, achado #3 (real) e um bug de frontend à parte

Depois do fix acima (`deadlinesForBroadcast`) ir pra produção e o CI verde, testei ao vivo de
novo com duas contas reais. Dois problemas distintos apareceram, com causas totalmente
diferentes:

### Bug de frontend: ring do hand-outcome preso em 0

O anel de contagem regressiva no botão "X" do banner de resultado de mão (a próxima mão já está
a caminho) simplesmente não aparecia. Achado em `ui/src/lib/hooks/useTableOutcome.ts`: o
`nextHandDurationMs` era recalculado dentro de um `useEffect`, que só roda **depois** do commit —
então a primeira renderização que carrega um `next_hand_unix_ms` genuinamente novo ainda via o
`nextHandArmed` da mão **anterior**, calculava `nextHandDurationMs = 0`, e `HandOutcomeRing`
captura seu `elapsedMs` **uma única vez, no mount** (`ui/src/components/table/HandOutcome.tsx`).
Esse `0` inicial fica congelado. Corrigido movendo o ajuste de estado pra dentro do corpo do
render (padrão oficial do React para "ajustar estado quando uma prop muda"), eliminando a janela
onde isso pode acontecer. Testes novos em `ui/src/lib/hooks/useTableOutcome.test.tsx`.

### Achado #3 (real, arquitetural): erosão pelo lado que não commitou

Jogando de novo, o timer do **seat** (não o hand-outcome) continuava parecendo "quase
instantâneo" mesmo depois do fix #2. Investigação nos logs de `armTurnTimer` (só logados quando
`resumed_from_persisted=true`, ver a nota de redução de volume abaixo) mostrou que praticamente
todo turno estava sendo "resumido" de um deadline persistido, e ocasionalmente com `remaining_ms`
bem menor do que a janela nominal (ex.: `remaining_ms: 730` quando `base_deadline_unix_ms ==
deadline_unix_ms`).

Cruzando os logs de `"table ws connected"` por `logStreamName` (processo `app` vs `app2`) da
própria mesa de teste:

```
14:54:06  jogador A conecta → app2
14:54:30  jogador B conecta → app2   (MESMO processo)
15:03:37  jogador A reconecta (reload de página) → app   (processo DIFERENTE de B)
```

No início os dois caíram por acaso no mesmo processo nginx round-robin — nesse caso
`broadcastAll()` roda tudo síncrono no mesmo `*Actor`, sempre com timer "fresco", e visualmente
tudo funciona. O `resumed_from_persisted=true` virou onipresente exatamente a partir do reload
que moveu a conexão do jogador A pro outro processo.

**Importante — o que isso NÃO é:** não existe ausência total de comunicação entre os dois
processos. `app.go`'s `broadcast` closure já publica cada snapshot por-viewer via
`api-commons/ws.Registry` (Redis Pub/Sub, chave `tableID#viewerID`), então o que o jogador **vê**
sempre esteve correto — nenhuma ação real foi perdida, nenhum turno pulado de verdade. O gap real
é que o `*Actor` **interno** de cada processo (o que de fato arma o `time.AfterFunc` que faz o
auto-fold valer) é um pedaço de estado **separado**, não sincronizado por esse mesmo broadcast —
só era atualizado no próximo gatilho de reload **não relacionado** daquele processo (um
`ReconnectCmd` pausado por `tableconn.SyncInterval` = 15s, ou uma ação própria do jogador daquele
lado). Isso é o que deixava a imposição real do timer atrasada em relação ao que já tinha sido
commitado, silenciosamente.

**Correção:** `internal/tablenotify` — publica um sinal leve "mesa X mudou" num canal Valkey
Pub/Sub compartilhado a cada `Actor.commit` bem-sucedido (fire-and-forget, timeout curto, nunca
bloqueia o commit real). `tablemanager.Manager.ListenForExternalChanges` assina esse canal uma
vez por processo e despacha `table.ExternalChangeCmd` pro `*Actor` local daquela mesa (se
houver), forçando `ensureLoaded(ctx, true)` + `broadcastAll()` imediatos. Usa o client Valkey cru
(`cacheBackend.(*cache.RedisBackend).Client()`) porque `cache.Backend` não expõe Publish/
Subscribe — mesmo padrão que `SetHandHookClaimer` já usa. Uma mesa sem `*Actor` local nesse
processo é ignorada silenciosamente; nunca cria um ator novo como efeito colateral da notificação
(isso criaria e abandonaria um `*Actor` pra toda mesa que qualquer outro processo tocar).

Testes: `internal/tablenotify/tablenotify_test.go` (degrade gracioso sem cliente, mesmo padrão
de `handhook_test.go`); `internal/table/changenotify_test.go`
(`TestCommitNotifiesChangeOnEverySuccess`, `TestHandleExternalChangeForcesReloadAndBroadcast`);
`internal/tablemanager/changelisten_test.go`
(`TestListenForExternalChangesDispatchesToTheMatchingLocalActor`,
`...IgnoresUnknownTables`). `go build/vet/test` do módulo `api` limpos.

### Nota: por que os logs de diagnóstico pararam de aparecer em todo arm

Os logs `INFO` temporários (`armTurnTimer`/`armNextHandTimer`) originalmente disparavam em
**toda** rearmagem, não só na "resumida". Isso derrubou o CI (`TestMultiServerFuzz` estourou o
deadline apertado do teste por volume de log síncrono — ver commit `fb54178`). Agora só logam
quando `resumed_from_persisted=true`, que é justamente o caso em investigação.

**Fora do escopo (ainda):** roteamento sticky por `table_id` no nginx (eliminaria a divisão de
processo na raiz) não foi feito — o `upstream {}` que faz o round-robin vive num script
compartilhado do repo externo `ctech-cdk`, fora do alcance desta sessão, e mexer às cegas lá
arrisca derrubar a API. O fix via Valkey fica inteiro neste repo e resolve o mesmo problema sem
esse risco.

## 📌 Terceira atualização — a causa real do ring do hand-outcome (achado #4)

O fix do achado #3 (`internal/tablenotify`) foi confirmado em produção via bundle JS baixado
diretamente do CDN (`d({deadline:y,snapshotAt:n})` chamado no corpo do render, sem `useEffect` —
exatamente o fix esperado). Mesmo assim o ring do hand-outcome continuava preso em
`animation-duration: 0s` em produção. Comparando `"table next hand timer armed"` de uma mesma
`hand_id` nos logs:

```
deadline_unix_ms=1788538370893   (primeiro arm)
deadline_unix_ms=1788538370943   (segundo arm, mesma mão, 50ms depois)
```

Causa raiz de verdade: `nextHandDeadlineForPersist()` (chamado de `commit()`, computa o valor que
vai pro DynamoDB) e `armNextHandTimer()` (chamado de `broadcastAll()` logo depois de todo commit,
computa o valor real armado + transmitido) cada um chamava `timeNowFunc()` **independentemente**
pra calcular "agora + 12s" na primeira vez que uma mão chega em `Complete`. Entre esses dois
pontos no código roda `commitOutcomeLogEntries` (múltiplos commits extras pra registrar
vencedores) e todo o resto do handler — tempo real suficiente (dezenas de ms) pra essas duas
chamadas de `timeNowFunc()` decidirem valores diferentes depois do arredondamento de
`UnixMilli()`. O cliente (`useTableOutcome.ts`) trava no PRIMEIRO `next_hand_unix_ms` que vê e só
aceita um valor posterior se ele bater **exatamente** — então essa mesma mão nunca convergia e o
ring ficava preso em zero pra sempre, não só por um tick.

**Correção:** `nextHandDeadlineForPersist()` agora guarda o valor recém-calculado em
`a.pendingNextHandDeadline` (o mesmo campo que `ensureLoaded` já usa pra “resumir” um deadline
persistido entre instâncias) em vez de só devolvê-lo. Qualquer chamada seguinte — seja outra
chamada de `nextHandDeadlineForPersist` dentro do mesmo `commitOutcomeLogEntries`, seja o
`armNextHandTimer` que roda depois — reaproveita esse valor exato ao invés de calcular um novo.
`armNextHandTimer` já limpa esse campo depois de consumir, então não há vazamento entre mãos.

Teste de regressão: `TestNextHandDeadlinePersistedBeforeArmMatchesWhatGetsArmed`
(`fleetstate_test.go`) — precisou de um relógio falso incremental (`timeNowFunc` avançando 30ms a
cada chamada) pra reproduzir de verdade; com um relógio real não-mockado num teste apertado a
diferença fica em microssegundos e o `UnixMilli()` disfarça o bug. Confirmado falhando sem o fix
(drift de 30ms) e passando com ele.

**Nota:** o mesmo padrão (duas chamadas independentes de `timeNowFunc()`, uma em
`turnDeadlineForPersist()` e outra em `armTurnTimer`) existe pro timer de turno normal, mas o
gap ali é de microssegundos (nenhum trabalho pesado entre commit e broadcastAll pra um Act comum),
então não há evidência de que cause o mesmo efeito visível — deixado de fora desta correção;
revisitar se aparecer evidência concreta.

## 📌 Quarta atualização — o mesmo bug, agora no timer de turno (achado #5)

Sessão de captura ao vivo com WebSocket instrumentado desde antes da conexão (sem `reload` no
meio, dessa vez), jogando duas mãos reais. Decodificando os 77 frames capturados, achei múltiplas
mensagens `state` com o **mesmo** `snapshot_version` e `hand_id`, chegando a poucos milissegundos
de diferença, cada uma com um `action_base_deadline_unix_ms` **diferente**:

```
version=7, current=99cae0fe, t=...034  → base - t = +1562ms  (saudável)
version=7, current=99cae0fe, t=...045  → base - t =  -696ms  (já vencido)
version=7, current=99cae0fe, t=...046  → base - t = +1550ms  (saudável de novo)
```

Três broadcasts pra mesma versão lógica de estado, chegando intercalados, um deles já vencido.
Como o cliente simplesmente renderiza o que chega por último, um cliente podia acabar mostrando
o deadline errado mesmo tendo recebido o certo momentos antes — exatamente o "consome time bank
instantaneamente" relatado.

**Causa raiz:** o mesmo bug do achado #4 (`nextHandDeadlineForPersist`/`armNextHandTimer`
chamando `timeNowFunc()` duas vezes independentes), só que no `turnDeadlineForPersist`/
`armTurnTimer` — e **agravado pelo próprio fix do achado #3** (`internal/tablenotify`): antes,
só o processo que processava a ação computava um deadline "fresco"; agora, toda mudança dispara
`ExternalChangeCmd` em **todo processo** que também está servindo essa mesa, e cada um roda seu
próprio `armTurnTimer` de forma independente. Sem os dois pontos (persist e arm) reaproveitarem
o mesmo valor, cada processo podia computar um "agora" ligeiramente diferente e publicar sua
própria versão pro mesmo canal Redis compartilhado — múltiplas fontes escrevendo a mesma
"verdade" com pequenas divergências.

**Correção:** mesmo padrão do achado #4, aplicado a `turnDeadlineForPersist`: o valor recém
calculado fica em `pendingPersistedDeadline`/`pendingDeadlineFor`/`pendingDeadlineForStage`
(os mesmos campos que `ensureLoaded` já usa pra resumir entre instâncias), então `armTurnTimer`
— desse mesmo processo — reaproveita o valor exato em vez de recalcular. Isso não elimina os
múltiplos broadcasts (ainda vêm de processos diferentes), mas garante que todos concordem no
mesmo número, então não importa qual chega por último.

Teste de regressão: `TestTurnDeadlinePersistedBeforeArmMatchesWhatGetsArmed`
(`fleetstate_test.go`), mesmo relógio falso incremental do achado #4. Confirmado falhando sem o
fix (drift de 30ms) e passando com ele.

**Ainda em aberto:** não investiguei se o MESMO padrão de múltiplos processos publicando o mesmo
`snapshot_version` com pequenas divergências pode acontecer em outros campos além dos deadlines
(ex.: `idle_removal_unix_ms`) — o fix aqui só fecha os dois relógios que já tínhamos evidência
concreta de problema.

## 📌 Quinta atualização — atraso de ~16-17s na entrega do WebSocket (achado #6)

Depois dos achados #4/#5 corrigidos e implantados, ainda restava um sintoma vivo: mesmo com um
único deadline correto sendo persistido e armado, o **primeiro** broadcast que carregava esse
deadline às vezes só chegava ao cliente 16-17 segundos depois do timer ter sido armado — o
suficiente pra o relógio do turno já estar todo consumido (ou o "next hand" ring já ter passado
da janela de 12s) no instante em que o cliente finalmente recebe o frame.

**Descartado, em ordem:** foco/throttling da aba Chrome (idêntico com e sem foco), clock skew do
cliente (~424ms, irrelevante), staleness de DNS pro Valkey (`cache.internal.aoctech.app`
resolvendo pro IP certo), reinício das duas instâncias `app`/`app2` (usuário previu corretamente
que não resolveria), throughput/PPS de rede e CPU (CloudWatch mostrando uso trivial).

Um log temporário no fechamento `broadcast` de `internal/app/app.go` provou que o gap de ~16.7s
fica 100% entre a chamada de `reg.Broadcast()` (1ms depois do timer ser armado) e o frame chegando
no cliente — ou seja, dentro do caminho Valkey Pub/Sub → `RedisRegistry`, fora do código deste
repo. Um teste raw com `redis-cli PSUBSCRIBE ws:*` + `PUBLISH` manual na instância EC2 de produção
mostrou entrega **instantânea** — descartando de vez o Valkey/rede em si como causa.

**Causa raiz:** um único `valkey.Client` (de `cache.NewRedisBackend`) era compartilhado entre
TODO o app — cache genérico, `presence`, `handhook`, `ratelimit`, e o próprio
`ws.RedisRegistry.Broadcast` / `internal/tablenotify`. O `valkey-go` multiplexa as chamadas
`Do()` desse client numa mesma pipe/conexão e entrega as respostas na ordem em que os comandos
foram enviados (head-of-line): se um comando qualquer — bulk read do `presence`, checagem de
`ratelimit`, etc. — entra na fila antes de um `PUBLISH` urgente do timer, esse `PUBLISH` só é
escrito no socket depois que os comandos à frente terminam. Nada falha, nada loga erro — só
fica na fila. Bate com o problema que o Artur lembrou do `ctech-dfe` envolvendo client/contexto
valkey compartilhado.

**Correção:** `internal/app/app.go` ganhou `newRealtimeValkeyClient`, um `valkey.Client`
dedicado, isolado do client de `newCacheBackend`, usado exclusivamente por
`ws.NewRedisRegistry` e `tablenotify.NewService`. Esse caminho de sinalização em tempo real
nunca mais compartilha fila/conexão com o tráfego de cache/presence/ratelimit. `handhook`
continua no client de cache — é uma checagem SET NX pouco frequente, não implicada nas
capturas ao vivo — mas pode ser revisitado se aparecer evidência.

O log temporário de diagnóstico ("table broadcast publish") foi removido de
`internal/app/app.go` depois de confirmada a causa raiz.

**Ainda não testado ao vivo:** o fix precisa ser reimplantado e reconfirmado com o usuário
jogando em ambas as contas, como nas rodadas anteriores.

## 📌 Sexta atualização — a causa raiz real era o relógio da EC2, não o Valkey (achado #7)

O achado #6 acima estava **contaminado**: a instância EC2 de produção (`i-07a59f22471db70c5`)
tinha o `chronyd` nunca sincronizado (`Leap status: Not synchronised`, toda fonte em
`Reach 0` — o `pool.ntp.org` padrão da imagem Alpine é inalcançável nesta VPC sem NAT/egress).
O relógio da instância estava ~18-23s atrasado e piorando (drift de 287 ppm acumulado ao longo
de ~16h de uptime desde a última substituição por Spot).

Como os deadlines (`action_base_deadline_unix_ms`, `action_deadline_unix_ms`,
`next_hand_unix_ms`) são timestamps absolutos calculados via `time.Now().Add(duração)` no
servidor, um relógio de servidor atrasado produz deadlines que já nascem "no passado" do
ponto de vista de qualquer cliente com hora certa — independente de qualquer latência de
entrega real. HARs confirmaram: ação→broadcast em ~220ms (saudável), mas
`action_base_deadline_unix_ms` já ~1.4s no passado; `next_hand_unix_ms` ~4.4s no passado.
Isso explica os dois sintomas completos: o "timebank instantâneo" (`Seat.tsx`'s
`showNormalClock` nunca fica verdadeiro porque `baseDeadlineMs > clockNow` já é falso na
chegada) e o "hand outcome ring nunca aparece" (`useTableOutcome.ts` deriva duração 0 de um
deadline já vencido).

O log de diagnóstico "table broadcast publish" (achado #6) media `time.Now()` do PRÓPRIO
processo com o relógio errado contra o timestamp de recebimento do navegador (relógio certo)
— o gap de ~16-17s medido era, na verdade, quase inteiramente o desvio do relógio do servidor
naquele momento, não trânsito real pelo Valkey Pub/Sub. O client dedicado (`newRealtimeValkeyClient`,
commits `e40f7b8`/`19d0c71`) continua sendo uma melhoria de isolamento válida por si só
(evita head-of-line blocking genuíno sob carga), mas **não foi a correção deste incidente**.

**Correção real:** `ctech-cdk`'s `assets/ec2-alpine/setup-base.sh` agora aponta o `chronyd`
para o Amazon Time Sync Service (`169.254.169.123`, link-local, alcançável sem NAT/egress) em
vez do pool padrão, remove qualquer `chrony.drift` obsoleto antes de reiniciar, e aguarda sync
(bounded, não-fatal). Ver `ctech-cdk`'s `docs/specs/2026-09-04-alpine-chrony-time-sync.md`.
Aplica-se a qualquer instância nova/substituída sem rebuild de AMI. A instância atual em
produção precisa ser drenada e substituída (nunca corrigir o relógio ao vivo com mesas
ativas — o salto pra frente venceria todos os deadlines persistidos de uma vez).

Os logs temporários de diagnóstico do achado #6 (`internal/table/actor_presence.go`,
`internal/tablemanager/manager.go`) foram removidos depois de identificada a causa raiz real.
