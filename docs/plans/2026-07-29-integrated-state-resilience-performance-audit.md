# Auditoria integrada — estado, erros, resiliência, performance e próximos produtos

Data: 2026-07-29  
Commit auditado: `0a8544b`  
Escopo: `api/`, `ui/`, `cdk/`, `.github/` e integração com
`2026-07-29-clubs-and-private-leagues.md` + `2026-07-29-multi-table-grid.md`

## Resumo executivo

O núcleo transacional do jogo está bem desenhado: o servidor é autoritativo, ações carregam versão e idempotência,
conflitos de DynamoDB forçam reload, snapshots escondem informação por viewer e o cliente tem ACK, timeout, resync e
watchdog. Os riscos mais importantes estão nas bordas:

1. **P0 — PRs de API não executam o workflow da API.** `.github/workflows/api.yml` só aceita
   `workflow_call`; `deploy.yml`, que o chama, não aceita `pull_request`. Uma alteração exclusivamente em `api/**` pode
   chegar ao merge sem unitários, integração ou race detector.
2. **P0 — `proto/**` não aciona API nem UI.** O filtro central conhece apenas `api/**`, `ui/**` e
   `cdk/**`. O contrato binário compartilhado pode mudar sem regeneração/teste/deploy de nenhum lado.
3. **P1 — refresh HTTP 401 não é single-flight.** O interceptor Axios chama `doRefresh()` diretamente, enquanto apenas o
   caminho WebSocket usa a coordenação de `session.ts`. Uma rajada de queries pode renovar simultaneamente a mesma
   sessão, repetir requests fora de ordem e emitir vários avisos.
4. **P1 — atualizações de estado durante render na mesa.** `setRememberedStart` e
   `setNextHandArmed` são chamados no corpo de `TableContent`; há padrões semelhantes em `Chat`,
   `RealityCheck`, `HandOutcome` e `AchievementToast`. Broadcasts rápidos geram render extra, tornam transições mais
   difíceis de raciocinar e são candidatos concretos a flicker.
5. **P1 — desligamento não está protegido no scale-in.** A aplicação drena actors e leases no
   `OnStop`, mas o CDK não instala lifecycle hook no ASG. O código correto pode não receber tempo para executar durante
   terminação de instância.
6. **P1 — os dois Schedulers não têm DLQ/retry policy.** Reconcile de dinheiro e limpeza de mesas usam EventBridge
   Scheduler sem `deadLetterConfig`, `retryPolicy` ou alarme. O archiver já tem DLQ; os schedulers ainda não.
7. **P1 — reconciliação faz `Scan` não paginado e sem índice de pendências.** Ao passar de 1 MB, movimentos pendentes
   deixam de ser vistos; antes disso, o custo cresce com resolvidos + pendentes.
8. **P2 — equidade roda dentro do actor e do fan-out por viewer.** O LRU reduz repetições, mas todo miss executa Monte
   Carlo síncrono no goroutine serial da mesa, antes de transmitir snapshots.

Ordem recomendada: corrigir CI/contrato; unificar política de autenticação/retry; remover state updates de render;
fechar lifecycle/DLQs/reconcile; só então otimizar o hot path de equity.

## 1. Frontend — autenticação, retries e estado

### F1 — 401 concorrente e sessão parcialmente encerrada (P1)

Evidência:

- `ui/src/lib/api/client.ts:102-114` marca `_retried`, chama `doRefresh()` e repete o request.
- `ui/src/lib/auth/session.ts:16-49` tem um boolean `refreshInFlight`, mas não uma Promise compartilhada, não devolve
  resultado e não é usado pelo interceptor.
- `TermsGate.tsx:31-44` e `useOptionalSession` também chamam `doRefresh()` diretamente.

Comportamento:

- 5 queries recebendo 401 podem disparar 5 refreshes.
- Se o refresh retornar `null`, o interceptor mantém o token morto em memória; a query rejeita e o toast diz para entrar
  novamente, mas a árvore autenticada não é necessariamente desmontada.
- Se o refresh falhar por rede, não há distinção central entre “offline” e “sessão revogada”.
- O request repetido não tem política para abortar quando a navegação desmonta o consumidor.

Correção:

- Substituir o boolean por `let refreshPromise: Promise<SessionResult | null> | null`.
- Expor uma única `getOrRefreshSession({clearOnUnauthorized})`, usada por Axios, `TermsGate`, sessão opcional e sockets.
- Em resposta conclusiva sem sessão: limpar token/username/playerId e emitir um evento de
  `session-expired`; o gate decide login/return URL.
- Em erro de rede: preservar credenciais, não criar loop e apresentar estado offline.
- Testar uma rajada de 401: exatamente um refresh e N requests repetidos; testar refresh nulo e refresh com falha de
  rede separadamente.

### F2 — retry HTTP genérico não respeita semântica (P1)

`QueryProvider.tsx:11-17` deixa o retry default do TanStack Query. Apenas sala/assento/buy-in excluem

404. Assim, queries podem repetir 401/403/404/429 e 5xx sem uma política comum. O interceptor já repete uma vez após
     401, podendo somar tentativas. `Retry-After` não é consumido.

Criar `shouldRetryQuery(failureCount, error)` e `retryDelay` centrais:

| Condição                        | Política                                                                      |
|---------------------------------|-------------------------------------------------------------------------------|
| 400/401/403/404/409             | não repetir query                                                             |
| 408/425/429                     | repetir somente operação idempotente; respeitar `Retry-After`; full jitter    |
| 500/502/503/504 e falha de rede | até 2–3 tentativas, exponential backoff + full jitter                         |
| mutation monetária              | nunca repetir automaticamente no browser; usuário repete com a mesma idem key |
| abort/cancel                    | nunca repetir nem notificar                                                   |

O Axios deve aplicar timeout explícito e normalizar `problem+json` (`type`, `title`, `detail`,
`request_id`) em um `ApiError`; hoje `notify.ts` reduz tudo ao status e perde contexto útil.

### F3 — state update durante render e flicker (P1)

Ocorrências confirmadas:

- `ui/src/app/table/page.tsx:220-229` — `setRememberedStart` durante render.
- `ui/src/app/table/page.tsx:347-349` — `setNextHandArmed` durante render.
- `ui/src/components/table/Chat.tsx:30` — contador de vistos durante render.
- `ui/src/components/table/RealityCheck.tsx:40-43`, `HandOutcome.tsx:145-148` e
  `AchievementToast.tsx:15-18` usam sincronização render → state.

Não é apenas estilo: a página recebe vários snapshots próximos (ação, equity, presença, reação, show-cards). Cada
`setState` durante render força nova passagem e pode apresentar por um frame o valor anterior. No grid, o custo e a
chance de transições visuais concorrentes multiplicam por quatro.

Mover sincronizações para effects orientados por chaves estáveis (`tableID:handID`, deadline,
`outcome.key`, `unlock.key`). Onde o valor é derivável de props, remover state. Para deadlines, armazenar a primeira
observação em `useRef`/reducer atualizado no handler do snapshot.

### F4 — reconexão preserva a mesa, mas zera demais na primeira abertura (P2)

`useTableRealtime.ts:469-483` limpa snapshot/chat/reactions quando `resetOnOpenRef` está armado; reconexões posteriores
preservam o último snapshot, o que é bom contra tela branca. A página, porém, substitui toda a mesa por loading quando
não há snapshot (`table/page.tsx:321-332`).

Melhorias:

- distinguir `initial-loading`, `reconnecting-with-stale-snapshot` e `terminal`.
- manter a última mesa visível, com veil não bloqueante, até receber snapshot autoritativo.
- em `forbidden`/`not_found`/`removed`, encerrar socket e navegar; hoje erro WS vira principalmente mensagem em
  `ActionBar`, mesmo quando não existe recuperação naquela mesa.
- persistir apenas o último snapshot **mascarado do próprio viewer** em memória de navegação, nunca em storage durável,
  para não ampliar exposição de cartas.

### F5 — resync de `rate_limited` mantém ação pendente sem reenvio (P2)

O cliente classifica `rate_limited` como resync e mantém `pendingActionRef`
(`useTableRealtime.ts:403-430`). Ele sincroniza estado, mas não reenvia a ação; a UI pode parecer “pendente” até o
snapshot/timeout resolver. A mensagem atual sugere aguardar e tentar novamente.

Escolher uma semântica única:

- preferível: rate limit rejeita definitivamente, limpa pending, respeita cooldown do servidor e habilita nova ação se
  ainda for o turno;
- não reenviar automaticamente uma aposta/fold após atraso, pois o contexto de decisão pode ter mudado.

### F6 — cache e invalidação (P2)

- `staleTime=60s` global é excessivo para sessão/assento e curto/irrelevante para catálogos.
- lobby recebe deltas WS, mas não invalida em reconnect; eventos perdidos enquanto offline podem deixar
  `rooms` defasado indefinidamente porque `refetchOnWindowFocus=false`.
- `room_updated` altera apenas `['rooms']`, não `['room', id]`.

Definir stale time por domínio e, no `onOpen` após reconnect do lobby, invalidar `rooms`, sessão aberta e saldo.
Atualizar ambas as chaves de sala no mesmo reducer. Para multi-mesa, não invalidar listas por snapshot: usar keys por
mesa e compartilhar apenas dados realmente globais.

## 2. Backend — resiliência e performance

### B1 — Scan de pendências monetárias não pagina (P1)

`api/internal/reconcile/pending.go:81-108` executa um único `Scan`, filtra no processo e ignora
`LastEvaluatedKey`. Após 1 MB, parte da fila nunca será reconciliada. Itens resolvidos continuam para sempre no conjunto
lido, salvo TTL externo não visível nesse modelo.

Correção recomendada:

- modelar `status=unresolved` + `next_attempt_at` em GSI esparso;
- claim condicional por item (`lease_until`, `attempts`) para execuções sobrepostas;
- backoff persistido, `last_error`, `resolved_at` e TTL para resolvidos;
- enquanto migra, paginar o Scan e limitar trabalho por invocação com checkpoint.

### B2 — dependência wallet sem retry/circuit breaker (P1/P2)

`walletclient.New` usa timeout total de 10 s. Chamadas são idempotentes, mas não há retry seletivo de falha transitória,
jitter, circuit breaker ou limites separados de connect/TLS/header. Em indisponibilidade, requests concorrentes podem
ocupar handlers por 10 s e amplificar a recuperação.

Adicionar transport com timeouts por fase, retry apenas de connect/reset/408/429/502/503/504 e apenas com idem key,
orçamento total curto, full jitter e breaker por endpoint. Emitir métricas de latência, status, retry e circuit-open.
Nunca repetir erro de saldo/regra.

### B3 — Monte Carlo no actor serial (P2)

`api/internal/table/actor.go:1795-1849` chama `equity.Estimate(..., 200)` para cada jogador no próprio fan-out.
`equity.go` tem LRU global de 20 mil entradas, mas:

- miss bloqueia comandos/timers/broadcast daquela mesa;
- cada viewer tem hole cards diferentes, portanto uma transição pode causar vários misses;
- o cache usa mutex exclusivo até no `Get` para mover a entrada da lista;
- não há métricas de hit/miss, duração ou evicção.

O primeiro passo é medir. Se p95 do broadcast/actor estiver afetado, calcular equity em worker pool limitado a partir de
um snapshot imutável, publicar mensagem `equity` apenas se
`snapshot_version` ainda for atual e descartar resultado velho. Isso já é suportado pelo guard do cliente. Evitar
goroutine por jogador sem limite.

### B4 — fan-out e clientes lentos (P2)

O actor chama `a.broadcast` sequencialmente para cada viewer. Há teste protegendo contra write físico síncrono, mas
devem existir métricas de queue/drop por conexão. Um consumidor lento nunca pode atrasar o actor; política recomendada é
mailbox limitada “latest snapshot wins”, enquanto ACK/erro usam canal prioritário e não descartável.

### B5 — concorrência de actors entre instâncias (P2)

O retry de conflito é deliberadamente uma vez (`actor.go:985-999`). Correto para não criar loop, mas conflitos
frequentes significam afinidade/lease degradada e elevam leitura + latência. Emitir
`VersionConflicts`, `ConflictRetrySuccess`, `ConflictRetryFailure` e alarme por taxa, não apenas
`LeaseFailovers`.

### B6 — limites e cancelamento (P2)

Fiber tem read/write/idle timeout, `Immutable` e panic recovery, pontos positivos. Faltam:

- `BodyLimit` explícito para HTTP;
- timeout contextual comum para DynamoDB em handlers;
- limites de resposta/listagem uniformes;
- propagação de cancelamento em hooks disparados após hand complete;
- readiness separada de uma checagem cara se o ALB chama `health-check` frequentemente.

## 3. CDK e operação

### I1 — lifecycle hook de scale-in (P1)

`tablemanager.Manager.DrainAndRelease` existe e o Fx `OnStop` o chama, mas `api-stack.ts` apenas cria o ASG. Adicionar
lifecycle hook `autoscaling:EC2_INSTANCE_TERMINATING`, heartbeat timeout suficiente, handler que manda o serviço parar
de aceitar novas conexões, drena e completa a lifecycle action. Configurar deregistration delay do target group alinhado
ao prazo.

### I2 — Scheduler DLQ/retry/alarme (P1)

`reconcile-stack.ts:73-77` e `tablecleanup-stack.ts:89-93` têm target apenas com ARN/role. Adicionar:

- SQS DLQ separada por job;
- permissão `sqs:SendMessage` para o Scheduler;
- `retryPolicy` com idade máxima e tentativas;
- alarmes de DLQ depth, Lambda errors/throttles e “job não executou”.

O archiver já usa `onFailure: new SqsDlq(dlq)`; documentação que ainda o chama de aberto está obsoleta.

### I3 — capacidade e HA (P2)

Produção permite 1–3 instâncias, mas `minCapacity: 1` mantém um único domínio de falha em regime normal. Para real
money, usar mínimo 2 em subnets/AZs distintas, target tracking por CPU + conexões/actor load, e synthetic canary de
HTTP + WebSocket auth/snapshot.

### I4 — WAF e observabilidade (P2)

**Decisão (2026-07-29): WAF não será adotado por custo.** A mitigação permanece nas validações, limites de payload e
rate limits da aplicação. A parte de observabilidade foi implementada com EMF e um dashboard CloudWatch único e
agregado (sem recursos por mesa, para limitar cardinalidade/custo), cobrindo:

- sucesso de ação e latência action→ACK;
- reconnects e tempo até snapshot;
- conflitos DynamoDB/throttling;
- pending cashouts por idade;
- actors, conexões e drops de mailbox;
- erro 401/429 por rota e versão da aplicação.

O dashboard também inclui saúde, latência, volume e 5xx do target group do ALB. A instrumentação usa templates de rota,
nunca IDs concretos, e ordena as dimensões EMF para manter identidades de métricas estáveis.

## 4. GitHub Actions — falhas de integração

### C1 — API sem CI em pull request (P0)

`api.yml` declara somente `workflow_call`. `deploy.yml` declara push e dispatch, não PR. Soluções:

1. adicionar `pull_request` diretamente a `api.yml`, com paths `api/**`, `proto/**` e o próprio workflow;
2. ou criar `ci.yml` central para PR que chama API/UI/CDK conforme filtros.

Separar teste de deploy evita conceder `id-token: write` ao job de teste; hoje os reusable workflows declaram permissões
de deploy no nível do arquivo.

### C2 — contrato protobuf fora dos filtros (P0)

Adicionar `proto/**` aos filtros de API e frontend, tanto em PR quanto push. Incluir gate que regenera Go e TypeScript e
falha se `git diff --exit-code` encontrar artefatos desatualizados.

### C3 — ordem de deploy por caminhos não cobre dependências (P1)

Uma mudança em API que exige UI compatível só marca API; o frontend não é reconstruído. O protocolo tem versionamento e
compatibilidade, o que reduz risco, mas mudanças em contrato/config compartilhado precisam acionar ambos. Criar grupo
`shared` (`proto/**`, scripts de geração, constantes de contrato)
que força API + frontend.

### C4 — gates complementares (P2)

- API: `go vet`, `staticcheck`, `govulncheck`/SCA e build de todos os `cmd`.
- UI: `tsc --noEmit` explícito antes de testes; o build não substitui diagnóstico rápido.
- CDK: rodar testes unitários/snapshot, não só diff e dois `grep`.
- Dependências: pin por SHA ações de terceiros sensíveis e habilitar atualização automática.
- Adicionar smoke pós-deploy e rollback/alarme; o deploy atual verifica SSM, mas não valida handshake WebSocket e uma
  ação idempotente de leitura depois do rollout.

## 4.1 Estado da implementação de hardening (2026-07-29)

O pacote de hardening foi aplicado sem WAF, conforme decisão de custo:

- **Frontend:** refresh de sessão é single-flight; erros HTTP usam um tipo normalizado e `Retry-After`; queries só
  repetem falhas transitórias; mutations não repetem implicitamente; eventos terminais de WebSocket encerram o reconnect;
  `rate_limited` libera a ação rejeitada; lobby e saldo são invalidados no reconnect; transições que retêm dados para
  animação foram movidas para efeitos guardados, sem atualização de estado durante render.
- **Backend:** `BodyLimit` explícito; métricas agregadas para HTTP, actors, conflitos, mailbox, equity e reconciliação;
  wallet com timeouts por fase, retry idempotente com jitter e circuit breaker; Scan legado paginado para não perder
  itens após 1 MB; resolvidos recebem TTL de 30 dias. A migração para GSI esparso/claim persistido permanece uma mudança
  de modelo a ser feita junto de backfill dos registros existentes, não uma condição de correção do Scan paginado.
- **Performance:** equity agora expõe duração, hit/miss e evicções. O worker pool descrito em B3 é deliberadamente
  condicional: só deve ser introduzido se o p95 medido mostrar bloqueio relevante do actor, evitando complexidade e
  resultados assíncronos sem evidência de gargalo. Mailboxes e conexões têm capacidade/backpressure observável.
- **Infra/operação:** scale-in usa lifecycle hook e drenagem; produção mantém no mínimo duas instâncias e target tracking;
  Scheduler de reconcile e cleanup possui retry, DLQ e quatro alarmes por job; dashboard operacional foi adicionado.
  Não há WAF. O canary autenticado de WebSocket exige uma identidade/segredo operacional externo e, portanto, não foi
  embutido no repositório; o deploy valida a release efetivamente servida por HTTP e faz rollback automático em falha.
- **CI/CD:** API roda também em PR, incluindo race/integration, vet, build, staticcheck e govulncheck; UI faz lint,
  type-check, cobertura e build; CDK compila e testa; protobuf é regenerado em gate próprio; mudanças compartilhadas
  acionam API e UI; Dependabot cobre Actions, Go e ambos os projetos npm; a action AWS com OIDC foi fixada por SHA.

Os alarmes adicionados são métricas padrão do Scheduler/Lambda/SQS. São oito alarmes no total para os dois jobs; pelo
preço público usual de alarmes padrão, o teto incremental fora do free tier é aproximadamente **US$ 0,80/mês**, além
do eventual custo do dashboard e das métricas customizadas EMF.

## 5. Integração com Clubs e Multi-table Grid

Os dois planos são compatíveis, mas criam requisitos cruzados que devem entrar antes da implementação:

1. **Club room identity no grid.** `TablePane` precisa receber metadados (`club_id`, nome curto, moeda) para distinguir
   notificações, reconnect e foco. Não depender apenas de `tableId`.
2. **Atalhos são P0 de segurança do grid.** Apenas o painel focado pode executar teclado/voz.
3. **Refresh 401 single-flight vem antes do grid.** Quatro sockets + queries multiplicam a rajada.
4. **Uma única sessão de jogo responsável.** Agregar tempo, mãos e exposição financeira entre painéis; clubs não podem
   reiniciar o relógio por sala.
5. **Lobby realtime como índice, não como autoridade de turno.** Cada socket de mesa já informa turno; o lobby deve
   avisar criação/fechamento, convite, waitlist e mudança de membership.
6. **Limites de produto no servidor.** Máximo de quatro mesas abertas, limites de clubes/membros e proibição de
   real-money club room devem ser server-side.
7. **Temporada imutável por mão.** Salvar `club_id` + `season_id` no início da mão; rollover no meio não pode atribuir
   resultado à temporada nova.
8. **Remoção de membro e mesa aberta.** Definir se a remoção bloqueia apenas nova entrada ou também força saída.
   Recomendação: não expulsar stack em mão; marcar saída segura ao fim e revogar novo join.

## 6. Features propostas

### 6.1 Central de sessão multi-mesa (alto valor, baixa/média complexidade)

Shell único mostrando mesas abertas, status (`sua vez`, reconectando, sentado fora), stack/P&L, clube/temporada e ação
“retomar”. É a união natural do grid com `ActiveTableBanner`, corrige a escolha arbitrária da primeira sessão e torna
falhas recuperáveis sem F5.

### 6.2 Waitlist e “seat ready” (alto valor)

Fila server-side idempotente para mesas cheias, com evento no lobby socket e prazo curto para buy-in. Para clubs, pode
ser restrita a membros; no grid, “seat ready” abre/foca um painel sem exceder quatro.

### 6.3 Club challenges e calendário de temporada (alto valor)

Objetivos sandbox como mãos jogadas, presença em noites do clube, variedade de adversários e conquistas coletivas.
Evitar premiar volume de aposta/perda. Usa achievements existentes e dá função à temporada sem introduzir premiação
financeira/regulatória.

### 6.4 Mesa “home game” recorrente (médio valor)

Preset de sala do clube (stake, lugares, timeout, run-it-twice, dia/horário), com criação manual no primeiro release.
Automação posterior só depois de Scheduler + DLQ + idempotência estarem resolvidos.

### 6.5 Replay/estudo compartilhado do clube (médio valor)

Membro compartilha mão redigida no feed do clube, com comentários em posições da timeline. Reutiliza hand
share/replayer; exige moderação simples e opt-in. Nunca revela hole cards além do que a prova permitiu.

### 6.6 Diagnóstico de conexão para o jogador (baixo custo)

Painel “qualidade da mesa” com RTT do heartbeat, reconnect count e último snapshot, acompanhado de copiar `request_id`
/session diagnostic sem token. Reduz suporte e ajuda a separar erro local de falha de servidor.

### 6.7 Modo observador (posterior)

Interessante para clubs, mas não deve ser acoplado ao grid inicial. Requer view server-side própria, delay
anti-collusion, limite de espectadores e nenhuma informação escondida. É uma feature de segurança/produto, não apenas
UI.

## 7. Plano de execução

### Fase 0 — bloquear regressões

- Corrigir triggers de PR e `proto/**`.
- Adicionar teste de geração protobuf limpa.
- Centralizar refresh/retry e testes de rajada 401.
- Remover state updates durante render.

### Fase 1 — durabilidade operacional

- DLQ/retry/alarmes dos Schedulers.
- Lifecycle hook + drain de ASG.
- Paginação imediata do reconcile e desenho do GSI/claim.
- Métricas de conflito, pending age, actor/broadcast e action→ACK.

### Fase 2 — performance comprovada

- Benchmark/load test com 1/2/4 mesas por usuário.
- Instrumentar equity LRU e actor stall.
- Worker pool versionado somente se as métricas justificarem.
- Cache policy por domínio e refresh do lobby após reconnect.

### Fase 3 — produto

- Implementar clubs com `season_id` fixado por mão e regras de membership.
- Implementar grid sobre `TablePane`, com teclado/voz/foco seguros.
- Entregar Central de sessão + waitlist.
- Depois: challenges, home games recorrentes e estudo compartilhado.

## 8. Critérios de aceite sistêmicos

- Uma rajada de 20 respostas 401 causa exatamente um refresh.
- 401 definitivo desmonta a sessão; falha de rede não apaga sessão.
- Nenhum 4xx permanente é repetido por TanStack Query.
- Nenhum componente atualiza state no corpo do render para sincronizar props.
- Mudança em `proto/**` testa e implanta API + UI, com generated code limpo.
- PR apenas de `api/**` executa unit, integration e race.
- Reconcile processa mais de 1 MB sem perder item e suporta duas invocações simultâneas.
- Falha repetida de Scheduler chega à DLQ e dispara alarme.
- Scale-in para de aceitar novas conexões e conclui drain antes da terminação.
- Quatro mesas não duplicam ação de teclado/voz, refresh ou reality check.
- Nenhum resultado assíncrono de equity é publicado para snapshot antigo.
