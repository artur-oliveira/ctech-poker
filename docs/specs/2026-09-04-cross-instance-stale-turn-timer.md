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
