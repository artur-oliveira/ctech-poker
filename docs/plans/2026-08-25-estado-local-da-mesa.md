# Estado local da mesa — auditoria e correções

**Data:** 2026-08-25
**Origem:** auditoria pedida depois das duas primeiras correções do mesmo dia
(selo de sequência em `internal/tablestreak` e expulsão indevida em
`handleKickTimeout`). A pergunta era se sobrava mais estado em memória local no
actor sujeito à mesma classe de erro.

## A classe de erro

Qualquer instância pode rodar um `Actor` para qualquer mesa
(`tablemanager.Manager.GetOrCreateActor`), e todas transmitem para os mesmos
sockets. Então todo estado que só existe na memória de um processo produz duas
respostas diferentes para a mesma coisa, alternando entre broadcasts, ou dispara
o mesmo efeito colateral duas vezes.

## O que estava correto (não mexer)

| Estado | Por quê |
|---|---|
| `cached` / `version` / `handID` | DynamoDB + CAS de versão |
| `activity` (chat/reações/preseleções) | persistido em `StoredTable.Activity` |
| `turnDeadline` / `turnBaseDeadline` | persistido em `TurnDeadlineUnixMs` |
| `winnerCardsArmedFor` | derivado do `req.ExpiresAt` persistido |
| `outcomeLoggedForHand` | `actionID` determinístico + `ErrDuplicateAction` |
| config de room (`turnTimeout`, `kickGrace`, `escalationCfg`, `equityEnabled`) | lida do `roomstore`, igual em toda instância |
| `activeConns` / `connCount` | sockets físicos vivem *neste* processo |
| `runoutTimer*`, `lastBroadcastStage` | só ritmo de animação; o CAS garante a correção |

## Correção 1 — hooks de fim de mão rodando duas vezes

**Sintoma:** `hands_played` andando de dois em dois, sequências pulando, unlocks
de tier emitidos em duplicidade.

**Raiz:** `Table.lastOutcome` faz parte do estado persistido
(`engine/hand/state.go`). Então *qualquer* instância que carrega uma mesa em
`Complete` e chama `broadcastAll` chega em `notifyHandComplete` — e
`broadcastAll` roda em chat, reação, connect e disconnect, não só na ação que
completou a mão. A única barreira era `Actor.completedHandNotified`, um campo
por processo. Os hooks a jusante (`achievements.RecordHand`,
`RecordTableStreak`, varredura de auto-rebuy) usam `Increment`/`IncrementStreak`
puros, sem guarda por mão.

Caminho reproduzível: mão fecha na instância A → jogador manda chat na
instância B durante os 12s de contagem → `commitActivity` faz `ensureLoaded`,
carrega `Complete` com `lastOutcome`, `broadcastAll`, hooks de novo.

**Correção:** `internal/handhook` concede o direito de rodar os hooks exatamente
uma vez por `(table_id, hand_id)`. Usa `SET NX` no cliente Valkey cru, não
`cache.Backend` — o par `Get`/`Set` não expressa um claim atômico, e um
read-then-write deixaria as duas instâncias observarem "sem dono".

O claim falha **aberto**: Valkey inacessível degrada para o comportamento
anterior (ao menos uma vez), porque pular os hooks perderia a progressão da mão
para sempre, enquanto um crédito dobrado é limitado e visível.

## Correção 2 — contagem pós-mão divergente

**Raiz:** `nextHandDeadline` era um `time.Time` em memória, sem contrapartida
persistida. Cada instância começava sua própria janela de 12s a partir do seu
próprio `now`, e só a que armou o timer emitia `next_hand_unix_ms` (a emissão é
guardada por `a.handID == a.nextHandArmedFor`) — clientes atendidos por
qualquer outra viam contagem diferente ou nenhuma.

**Correção:** `StoredTable.NextHandDeadlineUnixMs`, gravado no mesmo
`CommitAction` que completou a mão, e retomado por `armNextHandTimer` via
`pendingNextHandDeadline` exatamente como `armTurnTimer` retoma o relógio do
turno.

Aqui persistir ganha de cachear: o valor nasce junto com uma transição de
estado, então pega carona na conditional write que já existe — sem round trip,
sem TTL, sem degradação.

## Correção 3 — sequência de par de bolso

**Raiz:** `achievements.Service.lastPocketPair`, mapa com mutex por processo,
decidia se `KeySamePocketPairStreak` continuava ou resetava. Qualquer instância
pode servir a mão que completa, então a sequência avançava ou resetava conforme
quem serviu.

**Correção:** o valor passa para `cache.Backend`
(`poker:achv:lastpocketpair:<mode>#<playerID>`, TTL 30d). Sem cache o mapa local
continua sendo o fallback, o que mantém dev e instância única idênticos.
Falha de leitura é reportada como "sem par anterior", que reseta a sequência — a
mesma direção segura que o resto de `RecordHand` toma com dado faltante.

## Correção 4 — ponto de conexão do assento

**Raiz:** `applyPresence` só conhecia `disconnectedSince`/`activeConns` locais e
usava `"connected"` como default. Errado nas duas direções: marca local velha
mostrava desconexão fantasma de quem reconectou em outra instância, e uma
instância que nunca viu o socket mostrava como presente quem já saiu.

**Correção:** `internal/tableconn` (`poker:tableconn:<tableID>`) guarda a união.
`syncFleetConns` republica o conjunto desta instância e lê o merge de volta:
forçado em connect/disconnect, pacejado em `SyncInterval` (15s) nos demais.

O heartbeat fica em `ensureLoaded`, não em `broadcastAll`. Uma mesa onde todos
estão conectados e quietos não transmite nada, então pacejar só pelo broadcast
deixava a chave expirar e as outras instâncias marcavam a fileira inteira como
desconectada. `ensureLoaded` roda em qualquer comando, inclusive no
`ReconnectCmd` de um ping de keepalive.

`tableconn` é **só exibição**. A expulsão continua apoiada em `LastActionAt`
persistido — um cache que uma queda de Valkey pode zerar nunca pode justificar
remover um jogador.

## Regra que sai daqui

Escolher o store pelo que o estado **é**:

- claim atômico → cliente Valkey cru (`SET NX`); `cache.Backend` não expressa
- valor que nasce com uma transição de estado → mesma conditional write
- estado compartilhado só de exibição → `cache.Backend`
- handle de socket físico → fica local, não tem escolha

E tudo cross-instance falha **aberto**: degradar a exibição ou aceitar uma
duplicata limitada, nunca travar a goroutine do actor nem descartar progressão.

## Testes

- `internal/tableconn/tableconn_test.go` — duas instâncias convergem; retração
  por expiração; separação por mesa; ID vazio; serviço nil; valor corrompido.
- `internal/handhook/handhook_test.go` — sem cache o claim é concedido;
  `handID` vazio; chaves não colidem entre mãos nem entre mesas.
- `internal/table/fleetstate_test.go` — hooks uma vez só entre instâncias; claim
  perguntado uma vez por mão; falha de claim ainda credita; ponto de conexão
  segue a frota nas duas direções; socket local ganha de conjunto velho; falha
  de sync preserva a última resposta; pacing vs. forçado; `ensureLoaded` mantém
  a chave viva; contagem pós-mão retoma o valor persistido, persiste o que
  armou, e é limpa fora de `Complete`.
- `internal/achievements/service_test.go` — par de bolso compartilhado entre
  instâncias; mão não qualificada limpa; fallback local sem cache.

Verificação: `go build ./...`, `go vet ./internal/...`, `go test ./... -race`,
`go test -tags integration -race ./tests/integration -run
'TestMultiServerFuzz|TestDisconnectKick|TestNextHand|TestTurnDeadline'` e o
stress `-run TestMultiServerFuzz -count=15` exigido pelo `api/CLAUDE.md` para
mudanças nos caminhos de timer do actor.
