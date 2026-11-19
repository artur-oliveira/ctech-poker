# Web Push agendado: lembrete de ofensiva e "venha jogar hoje" (Poker) e pendências (Billing)

**Status:** aprovado em 2026-11-18; implementado e inerte (sem chaves VAPID nada é enviado) · **Data:** 2026-11-18 · **Base:** `ctech-dfe/docs/specs/2026-11-17-web-push.md`

## 1. Intenção

O DF-e já tem Web Push (go-common `webpush`, `@aoctech/push-client`, tabela `_push`, Lambda `push-sender`). Lá o push nasce de um
**evento**. Aqui nasce do **relógio**: o usuário escolhe um horário, ou o sistema escolhe um, e algo precisa acordar nessa hora.

| Produto | Push | Horário | Quando sai |
|---|---|---|---|
| Poker | Lembrete de ofensiva | escolhido pelo usuário | ofensiva > 0 e bônus de hoje ainda não resgatado |
| Poker | "Venha jogar hoje" | calculado: hora em que o usuário costuma jogar | ainda não jogou hoje e não recebeu o lembrete de ofensiva hoje |
| Billing | Lembrete de pendências | escolhido pelo usuário | há ocorrências de recorrência em `pending_confirmation` |

Spec do Billing: `ctech-billing/docs/specs/2026-11-18-pending-reminder-web-push.md`.

## 2. Objetivos, não objetivos

- Reaproveitar o que o DF-e construiu; **não** copiar `push.go` do DF-e para dois repositórios novos.
- Custo adicional abaixo de US$ 1/mês por produto com 10 mil usuários; nada sempre ligado, sem KMS novo, sem SQS
  (política de custo ocioso zero).
- Não objetivo: migrar o DF-e para o pacote compartilhado (fica como acompanhamento); push por evento; e-mail ou SMS.
- Anti-objetivo: spam. No máximo 1 "venha jogar" por dia por usuário, nunca se já jogou, e o usuário consegue desligar.

## 3. Abordagens consideradas

1. **Um schedule do EventBridge Scheduler por usuário.** Custo igual a zero, mas é preciso criar, mover e apagar milhares de
   schedules a cada mudança de preferência, e o "venha jogar" muda de horário sozinho. Descartada.
2. **Um schedule recorrente de 15 minutos + Lambda de tick + índice por faixa de horário (escolhida).** Um recurso fixo; a
   preferência é só uma linha no DynamoDB.
3. **Tick dentro da API (cron do líder).** Sem Lambda nova, mas acopla o envio ao ciclo de vida da frota e não escala a zero.
   Descartada.

EventBridge **Scheduler** (v2), não regras do EventBridge legado.

## 4. Decisões

### 4.1 Código compartilhado: novo pacote go-common `pushsvc` (minor `v1.44.0`)

Hoje o repositório, o serviço, as rotas de inscrição e o núcleo da Lambda de envio vivem em `ctech-dfe/api/internal` (cerca de
1.100 linhas). Copiar para Poker e Billing triplica isso. Proposta: extrair a parte que **não é DF-e** para `pushsvc`:

- `Store` (DynamoDB): inscrição `USER#{sub}` / `SUB#{sha256(endpoint)[:16]}`, contador `META#counts`, limite de 10 dispositivos,
  TTL de 60 dias renovado pelo sync.
- `Service`: `Subscribe`, `Devices`, `Unsubscribe`, `Config` (chave pública VAPID).
- `Mount(router, ...)`: rotas `/v1.0/push/config`, `/v1.0/push/subscriptions` (PUT, GET, DELETE `/:id`) e `/v1.0/push/test`.
- `SendJob` e `Sender.Handle`: núcleo da Lambda `push-sender` (lê os dispositivos do destinatário, envia, apaga o que está `Gone`).

Cada produto fica só com o que é dele: suas preferências, a regra de quando enviar e o texto. O DF-e continua como está até
alguém migrá-lo.

### 4.2 Horário em America/Sao_Paulo, sem fuso por usuário

A ofensiva do Poker já conta o dia em BRT (`cooldownKey`) e o Billing usa `brcal.Today()` em `America/Sao_Paulo`. Os dois
produtos são só do Brasil, então o horário do usuário é um "HH:MM" em BRT. Guardar fuso por usuário fica para quando houver
usuário fora do Brasil. (Escolha minha.)

### 4.3 Agendamento

- Um `aws_scheduler_schedule` / `aws.scheduler.CfnSchedule` com `cron(0/15 * * * ? *)` e `ScheduleExpressionTimezone =
  America/Sao_Paulo`, alvo: Lambda `push-tick` do produto. Faixas de 15 minutos: 96 por dia, numeradas 0 a 95.
- O tick calcula a faixa atual e processa também a anterior (recupera um disparo perdido; o marcador de envio do dia garante que
  nada sai duas vezes).
- Candidatos vêm de um GSI esparso `slot-index` (pk `SLOT#NN`, sk `{tipo}#{sub}`). Sem `Scan` em lugar nenhum.
- Cada candidato vira um `SendJob` já com texto pronto; a Lambda `push-sender` (de `pushsvc`) só envia.
- Padrão do Scheduler já existe no Billing: `terraform/billing/byok_warm.tf` (role `scheduler.amazonaws.com`, schedule, alvo
  Lambda).

### 4.4 Poker

**Tabela `poker_push`** (CDK, `cdk/dynamodb-stack.ts`): on-demand com teto, chave própria da AWS **desligada** (padrão da política
de custo), TTL, PITR só em prod. Itens: inscrição e contador (de `pushsvc`), mais:

| Item | pk | sk | Atributos | GSI `slot-index` |
|---|---|---|---|---|
| Lembrete de ofensiva | `USER#{sub}` | `REMIND#streak` | `enabled`, `slot`, `last_sent_day` | `SLOT#{slot}` / `streak#{sub}` |
| Venha jogar | `USER#{sub}` | `REMIND#play` | `enabled`, `slot`, `last_sent_day` | `SLOT#{slot}` / `play#{sub}` |

`last_sent_day` é atualizado com condição (`<> hoje`): garante no máximo um envio por dia por tipo mesmo com tick duplicado.

**Regras no tick, por candidato:**

- *Ofensiva:* `dailyreward.Service.Status` (1 `GetItem`) com `current_streak > 0` e `last_claim_day != hoje`. Texto sem valores
  de fichas: "Sua ofensiva de N dias termina hoje. Resgate o bônus." Link para o lobby.
- *Venha jogar:* pula se o usuário já jogou hoje (leitura no `sessionlog`) **ou** se o lembrete de ofensiva está ligado e a
  condição dele vale hoje (a ofensiva tem prioridade, sem depender da ordem dos horários).

**Horário do "venha jogar":** mediana da hora de início das últimas 14 sessões do `sessionlog`, arredondada para a faixa de 15
minutos. Com menos de 5 sessões, usa 19:00 BRT. Recalculado **depois de cada envio** (uma consulta ao `sessionlog`); se a
faixa mudou, a linha é regravada com o novo `slot`. Não há segundo schedule semanal.

**Preferências (API):** `GET/PUT /v1.0/push/preferences` com
`{"streak_reminder":{"enabled":bool,"time":"HH:MM"},"come_play":{"enabled":bool}}`.
Ao ativar o push no dispositivo: `come_play` começa **ligado** (decidido); `streak_reminder` começa **desligado** até o usuário
escolher um horário (escolha minha: é o horário que ele personaliza).

**Dados e LGPD:** `userpurge` e `userexport` passam a cobrir `poker_push`.

**UI** (`ui/`, export estático): `PushProvider` + tela de notificações com `@aoctech/push-client`, como no DF-e. Duas chaves
(ofensiva com seletor de hora, "venha jogar" liga/desliga) e o botão "Ativar neste dispositivo". Permissão pedida só no clique.
Nunca no carregamento.

### 4.5 Segredos

Par VAPID por serviço e ambiente em SSM SecureString (`/ctech-poker/{env}/push/...`), lido **uma vez na partida a frio** da
Lambda e mantido em memória (não decifrar em laço, política de custo ocioso zero). Gerado por um comando de keygen; reaproveitar o
`push-keygen` do DF-e movendo-o para o go-common.

## 5. Custo estimado

Premissas: 10 mil usuários, 30% ativam (6 mil dispositivos), até 1 push por dia por usuário. Poker: ~US$ 0,3 a 0,5/mês.
Billing: ~US$ 0,2/mês. Scheduler: 2.880 disparos/mês, dentro dos 14 milhões grátis. Linear: ~US$ 3 a 5 com 100 mil usuários no Poker.

## 6. Plano em fases

| Fase | Entrega | Verificação |
|---|---|---|
| F1 | go-common `pushsvc` `v1.44.0` (store, service, rotas, núcleo do sender) | testes unit + integração com DynamoDB local, `-race` |
| F2 | Poker API: `poker_push`, `push-tick`, `push-sender`, preferências, Scheduler (CDK) | testes das regras (ofensiva, jogou hoje, prioridade, 1/dia), integração |
| F3 | Poker UI: tela de notificações, SW | vitest + e2e do fluxo de ativação |
| F4 | Billing (spec própria) | |
| F5 | Skill `ctech-dev-standards`: push agendado | |

Cada fase sai inerte (sem chaves VAPID o `push-sender` não faz nada, como no DF-e) e só liga depois de o operador gravar as
chaves e conferir o job de Deploy.

## 7. Em aberto para quem constrói

- Como o `sessionlog` responde "jogou hoje" e "início das últimas 14 sessões" em uma consulta; se não houver, guardar
  `last_played_day` e o histórico de horas na própria linha `REMIND#play`.
- Escopos OAuth do Poker para `/push/*` (seguir a família `notifications` do DF-e).
- Subir o go-common do Poker de `v1.40.1` e do Billing de `v1.42.1` para `v1.44.0`.
