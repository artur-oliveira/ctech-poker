# Plano de Implementação — Otimização de Performance, Resiliência e Custos (`ctech-poker`)

**Data:** 2026-07-28  
**Escopo:** Backend Go (`api/`), Frontend React/Next.js (`ui/`) e Infraestrutura AWS CDK (`cdk/`).  
**Objetivo:** Consolidar os achados da auditoria em tarefas executáveis organizadas por prioridade, visando ganho de
performance, máxima resiliência operacional e redução de custos em nuvem.

---

## 📌 Nota de Arquitetura: VPC Endpoints vs. PrivateLink vs. IPv6

> [!IMPORTANT]
> **Esclarecimento de Infraestrutura e Custos (VPC Endpoints):**
> 1. **VPC Gateway Endpoints (DynamoDB e S3)**: São **100% GRATUITOS** (sem custo fixo por hora e sem taxa de
     transferência de dados por GB). Operam diretamente através da tabela de rotas da VPC.
> 2. **PrivateLink / Interface Endpoints**: São baseados em ENI (Elastic Network Interface) e possuem custo
     (~$7,20/mês por AZ + $0,01/GB processado). **NÃO utilizar PrivateLink**.
> 3. **Arquitetura Atual**: O serviço usa `PrivateIpv4Ec2Service` sem NAT Gateways (economia de ~$32/mês por NAT + $
     0,045/GB). O tráfego de saída utiliza IPv6 / Dual-Stack (`AWS_USE_DUALSTACK_ENDPOINT=true`). Os Gateway Endpoints
     gratuitos garantem roteamento direto interno para DynamoDB/S3 pela rede AWS sem encarecer a infraestrutura.

---

## Fase 1 — Resiliência & Correções Críticas (Prioridade Alta)

### T1 — Proteção contra DoS e Frame Racing no WebSocket (Backend)

- **Objetivo**: Evitar que clientes lentos travem a goroutine `Run` da mesa e prevenir data races entre mensagens de
  dados e pings de controle.
- **Arquivos**: [
  `api/internal/api/v1/tablews.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/api/v1/tablews.go)
- **Passos**:
    1. Garantir `w.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))` em todas as escritas de mensagens em
       `wsConnAdapter.WriteMessage`.
    2. Adicionar o método thread-safe `WritePing()` em `wsConnAdapter` que adquire a trava `w.mu` antes de enviar o
       `PingMessage` no `startHeartbeat`.
- **Verificação**: `go test -race ./internal/api/v1/...` e teste com cliente WS simulando lag de rede.

### T2 — Shutdown Gracioso com Draining Completo de Atores (Backend)

- **Objetivo**: Garantir que requisições e commits no DynamoDB em andamento terminem antes da instância encerrar no
  SIGTERM.
- **Arquivos**: [
  `api/internal/app/app.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/app/app.go), [
  `api/internal/tablemanager/manager.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/tablemanager/manager.go)
- **Passos**:
    1. Atualizar `manager.DrainAndRelease(ctx)` para coletar o canal `<-actor.Done()` de todos os atores ativos.
    2. Bloquear a saída da função até que todos os atores tenham finalizado o processamento de suas filas `cmds` e
       commits no DynamoDB ou até o timeout do contexto de shutdown.
- **Verificação**: Enviar `SIGTERM` durante um loop de ações de mesa e validar no log que a mesa drenou antes do
  encerramento.

### T3 — Prevenção de Perda de Saldos na Limpeza de Mesas (`tablecleanup`)

- **Objetivo**: Impedir que uma mesa seja arquivada e deletada se o reembolso de algum jogador falhar.
- **Arquivos**: [
  `api/cmd/tablecleanup/main.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/cmd/tablecleanup/main.go)
- **Passos**:
    1. Adicionar uma flag `hasRefundErrors := false` no loop de participantes da mesa inativa.
    2. Se `mover.Credit` falhar, definir `hasRefundErrors = true` e pular a chamada de `stale.MarkArchived`. A mesa
       permanecerá inativa para nova tentativa na varredura seguinte.
- **Verificação**: `go test ./cmd/tablecleanup/...` com mock de falha no wallet client.

### T4 — Agrupamento Correto por Partição no Archiver Lambda

- **Objetivo**: Corrigir a partição de arquivos no S3 para eventos de múltiplas mesas entregues no mesmo batch do
  DynamoDB Stream.
- **Arquivos**: [
  `api/cmd/archiver/main.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/cmd/archiver/main.go)
- **Passos**:
    1. Refatorar `buildBatch` para agrupar as entradas de `e.Records` por `(table_id, hand_id)`.
    2. Gerar objetos S3 distintos para cada partição de mesa presente no batch.
- **Verificação**: `go test ./cmd/archiver/...` validando a criação de múltiplas chaves S3 quando o stream contiver
  eventos heterogêneos.

---

## Fase 2 — Performance & Eficiência de CPU/Memória (Prioridade Média)

### T6 — Redução de Latência de Leitura no DynamoDB (`LoadTable`)

- **Objetivo**: Substituir chamadas transacionais por leituras simples fortemente consistentes.
- **Arquivos**: [
  `api/internal/tablestore/dynamo.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/tablestore/dynamo.go)
- **Passos**:
    1. Em `LoadTable`, trocar `TransactGetItems` por `GetItem` especificando `ConsistentRead: aws.Bool(true)`.
    2. Manter `TransactWriteItems` para gravações condicionais (corretude de estado).
- **Verificação**: `go test ./internal/tablestore/... -race`. Medir a queda na latência de leitura (~50%).

### T7 — Otimização do Simulador de Equity Monte Carlo

- **Objetivo**: Reduzir uso de CPU no servidor e eliminar chamadas bloqueantes a CSPRNG.
- **Arquivos**: [
  `api/internal/table/actor.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/table/actor.go), [
  `api/internal/engine/equity/equity.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/engine/equity/equity.go)
- **Passos**:
    1. Substituir `crypto/rand` por `math/rand/v2` semeado com gerador local no loop de Monte Carlo.
    2. Ajustar iterações de simulação para 150–200 (suficiente para precisão visual).
    3. Adicionar controle de goroutine com cancelamento de contexto: se uma nova ação gerar uma nova versão de snapshot,
       cancelar cálculos de equity anteriores em andamento.
- **Verificação**: Benchmarks em `internal/engine/equity` (`go test -bench=.`).

### T8 — Fine-Grained Locking no Manager de Tabelas

- **Objetivo**: Impedir que o carregamento I/O de uma mesa bloqueie o lookup/criação de outras mesas.
- **Arquivos**: [
  `api/internal/tablemanager/manager.go`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/api/internal/tablemanager/manager.go)
- **Passos**:
    1. Remover a retenção de `m.mu` durante chamadas `m.store.LoadTable` / `SeedTable`.
    2. Utilizar `singleflight.Group` para garantir que apenas uma requisição carregue/crie uma determinada mesa por vez.
- **Verificação**: `go test -race ./internal/tablemanager/...`.

### T9 — Memoização de Componentes React & Derivação de Estado (Frontend)

- **Objetivo**: Reduzir re-renderizações da mesa a 60 FPS fluidos.
- **Arquivos**: [
  `ui/src/app/table/page.tsx`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/ui/src/app/table/page.tsx), [
  `ui/src/components/table/*`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/ui/src/components/table)
- **Passos**:
    1. Envolver componentes pesados (`Seat`, `PlayingCard`, `ChipStack`, `ActionBar`, `Board`) com `React.memo`.
    2. Envolver dados computados (`actionState`, `playerNotesByID`, `rotatedSeats`) em `useMemo`.
    3. Estabilizar callbacks passados aos assentos com `useCallback`.
- **Verificação**: Profiler do React DevTools confirmando zero renders desnecessários em componentes inativos durante
  updates de timer/chat.

### T10 — Gerenciador de Áudio com Pool de Buffers (Frontend)

- **Objetivo**: Eliminar latência de som e criação de elementos `Audio` no DOM.
- **Arquivos**: [`ui/src/lib/sound.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/ui/src/lib/sound.ts)
- **Passos**:
    1. Implementar um cache de elementos de áudio / Web Audio API buffer pool pré-carregados.
    2. Reutilizar instâncias de áudio em vez de instanciar `new Audio(file)` a cada evento.
- **Verificação**: Testar efeitos de som de fichas e cartas no navegador verificando latência zero e uso estável de
  memória.

---

## Fase 3 — Otimização de Custos & Ajustes Finais de Infra (Prioridade Média/Baixa)

### T11 — Remoção/Elevação do Teto no DynamoDB On-Demand & Projeção de GSIs

- **Objetivo**: Prevenir throttling em picos de tráfego e reduzir custo de armazenamento nos índices.
- **Arquivos**: [
  `cdk/lib/dynamodb-stack.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/cdk/lib/dynamodb-stack.ts)
- **Passos**:
    1. Remover ou ajustar `maxReadRequestUnits` / `maxWriteRequestUnits` nas tabelas principais.
    2. Trocar `ProjectionType.ALL` para `ProjectionType.KEYS_ONLY` ou `INCLUDE` nos GSIs de `poker_rooms`,
       `poker_leaderboard_stats` e `poker_player_hands`.
- **Verificação**: `cdk diff` confirmando alteração dos índices e billing parameters.

### T12 — Configuração do TanStack Query & Otimizações do Next.js (Frontend)

- **Objetivo**: Eliminar requisições REST redundantes no foco da janela e otimizar bundle JS.
- **Arquivos**: [
  `ui/src/lib/providers/QueryProvider.tsx`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/ui/src/lib/providers/QueryProvider.tsx), [
  `ui/next.config.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/ui/next.config.ts)
- **Passos**:
    1. Configurar `defaultOptions: { queries: { staleTime: 60 * 1000, refetchOnWindowFocus: false } }` no `QueryClient`.
    2. Adicionar `experimental: { optimizePackageImports: ['lucide-react'] }` em `next.config.ts`.
- **Verificação**: `npm run build` na pasta `ui/` e inspeção do painel Network no navegador ao alternar abas.

### T13 — Retenção de Logs no CloudWatch e DLQs em Schedulers (Infra)

- **Objetivo**: Evitar custos acumulados de logs antigos e alertar sobre falhas em cron jobs.
- **Arquivos**: [
  `cdk/lib/archiver-stack.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/cdk/lib/archiver-stack.ts), [
  `cdk/lib/reconcile-stack.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/cdk/lib/reconcile-stack.ts), [
  `cdk/lib/tablecleanup-stack.ts`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/cdk/lib/tablecleanup-stack.ts)
- **Passos**:
    1. Definir `logRetention: logs.RetentionDays.ONE_MONTH` para todas as funções Lambda.
    2. Adicionar SQS Dead-Letter Queue e `retryPolicy` nas definições de `CfnSchedule` em `reconcile` e `tablecleanup`.
- **Verificação**: `cdk diff`.

---

## 📊 Resumo de Impacto Estimado

| Fase                     | Esforço | Impacto na Performance              | Impacto na Resiliência                    | Impacto Financeiro                    |
|--------------------------|---------|-------------------------------------|-------------------------------------------|---------------------------------------|
| **Fase 1 (Resiliência)** | ~2 dias | Elimina travamentos na mesa         | 🛡️ Alta (Zero perda de dados e dinheiro) | Previne prejuízos e indisponibilidade |
| **Fase 2 (Performance)** | ~3 dias | ⚡ >80% redução de CPU / UI a 60 FPS | 🛡️ Isolamento de goroutines              | Permite reduzir tamanho de EC2        |
| **Fase 3 (Custos)**      | ~1 dia  | ⚡ Roteamento interno sem latência   | 🛡️ Alertas em falhas de cron             | 💰 -60% custos Dynamo/CloudWatch      |

---

## 🔮 TODO Futuro (Backlog de Escala & Alta Disponibilidade)

### T-FUTURO — Alta Disponibilidade Multi-AZ & Auto-Scaling (`cdk/lib/api-stack.ts`)

- **Status**: **Adiado / TODO Futuro** (A infraestrutura atual opera de forma estável com 1 instância EC2/ASG; aumentar
  a capacidade mínima para 2 instâncias adicionaria custo fixo desnecessário no momento).
- **Gatilho para Implementação**: Quando o volume de concorrência simultânea de jogadores ultrapassar a capacidade de 1
  instância ou quando for exigido SLA com garantia Multi-AZ contratual.
- **Passos para Quando For Implementar**:
    1. Alterar a capacidade mínima no ASG: `minCapacity: isProd ? 2 : 1` em [
       `cdk/lib/api-stack.ts:388`](file:///home/artur/Documents/Projects/Ctech/ctech-poker/cdk/lib/api-stack.ts#L388).
    2. Configurar a política de Target Tracking por CPU no CDK:
       ```typescript
       service.autoScalingGroup.scaleOnCpuUtilization('CpuScaling', { targetUtilizationPercent: 70 });
       ```

