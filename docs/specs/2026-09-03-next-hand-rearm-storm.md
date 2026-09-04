# Incidente + Correção — storm de "próxima mão" numa mesa travada gerou US$ 0,44 de DynamoDB

Data: 2026-09-03 · Escopo: `api/internal/table`, `api/internal/tablestore`, `cdk/lib/dynamodb-stack.ts`

## 📌 Sintoma na fatura

DynamoDB do `ctech-poker` pulou de ~US$ 0,03/dia para **US$ 0,44** no dia 2026-09-02, sendo
que só houve jogo entre ~12h e ~13h (BRT). Quebra por operação:

| Operação | 09-01 | 09-02 |
|---|---|---|
| WriteRequestUnits | US$ 0,033 | **US$ 0,423** |
| ReadRequestUnits | US$ 0,004 | US$ 0,013 |
| `us-east-1-KMS-Requests` (SSE) | US$ 0,011 | **US$ 0,068** |

Nada foi throughput orgânico. O throttle alarm (#34) **não disparou** — o on-demand escalou o
storm sem problema, só cobrou por ele.

## 📌 O que aconteceu (CloudWatch, BRT)

- **~12:10** — a instância spot `t4g.nano` do poker foi reciclada (`termination-drain` rodou
  12,8 s). A instância nova recarregou a mesa `01M1HABKV5Z90SGHZ63K44EGHB` do DynamoDB com um
  `next_hand_deadline_unix_ms` **já vencido** e o assento do jogador `78fd4d57` em
  `pending_exit=true` — o mesmo assento wedgeado da colisão de chave de settlement corrigida em
  `6bf8bd0` (`docs/specs/2026-09-03-system-leave-settlement-key-collision.md`).
- **13:03:58 → 13:15:27** (~11,5 min) — **5.779** ocorrências de
  `WARN table next hand dispatch failed … err:"table: refusing to commit duplicate seat for
  player 78fd4d57"`, ≈ 8/s, numa mesa só, um jogador só. Volume de log do app subiu de ~50/min
  para ~460/min.
- DynamoDB nesses 15 min:

  | Tabela | pico (5 min) | baseline de jogo |
  |---|---|---|
  | `poker_table_state` | **217.752 WCU** (~700 writes/s) | ~2.000 |
  | `poker_action_log` | 18.146 | ~350 |
  | `poker_pending_cashouts` | 18.140 (idêntico ao action_log) | ~0 |
  | `poker_table_state_history` | 11.315 | ~50 |

  Total ≈ 490k + 44k + 44k + 29k ≈ **607k WCU** + ~70k RCU de `ensureLoaded`. A US$ 0,625/M WCU +
  US$ 0,125/M RCU do on-demand novo ≈ **US$ 0,39**; KMS +US$ 0,057 são as `GenerateDataKey`/
  `Decrypt` do SSE para essas mesmas escritas. ≈ **US$ 0,44**.

`reconcile` mostrou no máximo **2 linhas** de `pending_cashouts` de fato persistidas (zeradas às
16:14 UTC) — os 44k são tentativas de `TransactWriteItems` **canceladas**, cada uma ainda faturada
a 2× WCU por item. Zero impacto em carteira, zero crédito duplicado.

## 📌 Causa raiz — duas camadas

1. **Gatilho:** o assento não saía (colisão de chave `#system_leave#exit_requested`), então a mesa
   ficava presa em `Complete` com deadline vencido. Corrigido em `6bf8bd0` (nonce por remoção).

2. **Amplificador — `armNextHandTimer` sem teto.** `67ed747` (#136) fez `handleNextHand` limpar
   `nextHandArmedFor` na entrada para permitir auto-cura de uma falha transitória. Efeito colateral:
   depois que o timer dispara uma vez, a checagem de idempotência `a.handID == a.nextHandArmedFor`
   deixa de segurar, e **todo** `rearmTimersFromCache` seguinte (reconnect, ping de keepalive, AFK
   sweep) re-arma um timer novo. Com `pendingNextHandDeadline` no passado, `delay` vira `0` e o
   dispatch é imediato. O cliente do `78fd4d57` estava num loop de reconexão (HARs
   `sair_mesa_sozinho.har`, `quebrou_as.har`) → ~8 dispatches/s → ~8 `TransactWriteItems`
   rejeitados/s (`commit` recusa "duplicate seat"), cada um faturado.

   `retryNextHand` já era limitado (`MaxNextHandRetries=5`), mas ele re-arma via `time.AfterFunc`
   direto, **não** passa por `armNextHandTimer` — o loop de storm não é o caminho do `retryNextHand`.

## 📌 Correção

### 1. Teto de re-arms por mão (`api/internal/table`)

`armNextHandTimer` passa a contar `(re-)arms` por `handID` (`nextHandArmGuardHand` /
`nextHandArmsForHand`). Past `MaxNextHandArmsPerHand = 12` (`turntimeout.go`), o timer é deixado
**des-armado** e um `slog.Error` é emitido uma vez. Os contadores zeram quando `a.handID` muda
(uma mão de fato começou) ou a mesa sai de `Complete` (`armNextHandTimer(false)`). Recuperação de
uma mesa nesse estado: `cmd/tablecleanup`, um operador, ou o `request_exit` já consertado — não
uma transação que continua sendo rejeitada.

Resultado: no cenário do incidente, ~12 dispatches por mão travada em vez de 5.779 — redução de
~480×, o custo teria sido < US$ 0,001.

Testes: `nexthand_test.go::TestArmNextHandTimerStopsReArmingAfterTheCap` (para no cap, timer
des-armado, guard reseta com `handID` novo) e assertion adicionada em
`TestArmNextHandTimerClearsWhenNotComplete`.

### 2. TTL + PITR-off nas tabelas efêmeras (`api/internal/tablestore`, `cdk`)

- `poker_table_state` e `poker_table_state_history` ganham `ttl` (`stateTTLDays = 7`). O item da
  mesa é `SET ttl` em **todo** `CommitAction` (junto do `last_action_at`), então uma mesa viva
  nunca expira; uma morta é reapada em 7 dias em vez de ficar para sempre. `SeedTable` também
  grava `ttl`. `SaveTableStateHistory` idem. `ttl` é palavra reservada em expression → alias
  `#ttl` no `UpdateExpression` (os encoders de `PutItem` não precisam).
- PITR desligado (per-tabela, via novo parâmetro `pitr` no helper `table()`) para:
  `poker_table_state`, `poker_table_state_history`, `poker_action_log` (S3 já é a cópia durável),
  `poker_action_guards` (crumb de idempotência de 7 dias), `poker_player_sessions` (presença por
  conexão). PITR fica **ligado** para tudo que é durável e não-reconstruível: `poker_player_hands`,
  `poker_*_stats`, `poker_*_matchups`, achievements, leaderboard, compras, `poker_pending_cashouts`
  (money-safety), `poker_rooms` (config de sala privada), `poker_player_notes`, `poker_social_*`,
  `poker_player_reports`.

Testes CDK: `dynamodb-stack.test.ts` — TTL em `poker_table_state`/`_history` e novo teste
`PITR is on for durable data and off for ephemeral tables`. Integração:
`tablestore/dynamo_test.go` assere `loaded.TTL` ~7 dias à frente após commit e no snapshot de
history.

### 3. Alarme de volume de escrita (`cdk`)

`addWriteVolumeAlarm` novo, em `ConsumedWriteCapacityUnits` (soma, janela de 5 min), **1 breach
já dispara** (`evaluationPeriods: 1`). Uma mesa six-max ativa faz ~8k WCU/5min; o incidente bateu
>200k. Thresholds: `poker_table_state` = 40.000, `poker_pending_cashouts` = 5.000 (baseline
orgânico ≈ 0, qualquer volume sustentado ali é loop). Wired no mesmo tópico SNS
`ctech-prod-alerts` (nunca um tópico novo — #34), gated em `cloudwatchAlarmsEnabled`. Teria
disparado em ~5 minutos.

### 4. Circuit breaker por mesa no `CommitAction` (#207, 2026-09-04)

O teto do item 1 conserta *aquele* loop. `tablestore.CommitAction` continua sendo o sink de escrita
compartilhado por todo comando, timer e sweep do processo, então `internal/tablestore/breaker.go`
adiciona a defesa em profundidade que ele deve a todos os outros chamadores.

**A condição de trip é a forma do storm, não a taxa:** `maxConsecutiveRejections` = 32 commits
rejeitados (version conflict ou duplicate action) **sem nenhum commit aceito no meio** abrem o
circuito da mesa — uma escrita condicional rejeitada significa que a mesa não avançou, logo
repetir a mesma mutação também não pode dar certo. Qualquer commit aceito zera a contagem. Com o
circuito aberto nada chega ao DynamoDB e o chamador recebe `ErrCommitThrottled`, que **envolve
`ErrUnavailable`, nunca `ErrVersionConflict`** (o actor tem que abortar o comando; um erro com cara
de conflito seria respondido com o reload-and-retry imediato que é justamente o loop a evitar).
Recuperação: cooldown de 2s dobrando até 60s por probe half-open falhada, uma probe por vez. No
cenário deste incidente: ~45 transações em vez de 5.779 (~130×), trip em ~4s.

**Token bucket por mesa foi tentado e descartado de propósito:** taxa de commit não é sinal, porque
só o jogo real é pausado por gente. O teste de integração nine-handed do `internal/table`
(`TestNineHandedTableGrowsPlaysPausesAndLeaves`) sustenta ~115 commits/s numa mesa — ~14× os ~8/s
do incidente. Logo qualquer teto que pegasse o incidente estrangula tráfego legítimo (e estrangulou,
duas vezes, em duas calibrações), e qualquer teto que não incomode o tráfego legítimo é alto demais
para limitar a fatura. Tetos por tipo de comando continuam onde o pacing é conhecido: os caps de
timer/retry do próprio actor.

Logs: uma linha por transição de estado (`table`, `action`, `cause`, `cooldown_ms`), nunca por
tentativa — o sintoma deste incidente foi 5.779 linhas WARN para uma mesa só. Não há
`internal/metrics` no serviço e nenhum coletor ad-hoc foi criado; o `addWriteVolumeAlarm` do item 3
segue sendo o sinal numérico.

Testes (clock fake, sem load test): storm, jogo em velocidade de máquina, contenção que ainda
progride, outage do store (não abre o circuito), isolamento por mesa, evicção de mesas idle —
`api/internal/tablestore/breaker_test.go`.

## 📌 A dúvida "os commits de hoje resolvem parcialmente?"

Não parcialmente para *este* incidente. `6bf8bd0` (hoje) mata o **gatilho** — aquela mesa não
trava mais do mesmo jeito. `67ed747` (2026-09-02, já estava em prod ontem) foi o que transformou
um stall silencioso num loop caro. Esta correção adiciona o **teto** no amplificador: mesmo que
um *outro* bug futuro trave uma mesa em `Complete`, o custo é limitado a ~12 transações + um
alarme em 5 min, não uma fatura.

## 📌 Limpeza de dados

Nenhuma migração. Ligar TTL numa tabela sem o atributo é no-op para itens existentes — eles só
nunca expiram (comportamento atual). Itens novos passam a carregar `ttl`. Desligar PITR é
não-destrutivo (para os backups contínuos; a janela existente se esvazia após a retenção).
