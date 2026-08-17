# Plano de implementação — Pessoas, amizade, segurança e jogadores recentes

Data: 2026-08-16  
Status: em implementação; PRs 1–5 (backend social, presença/recentes e denúncias/operação) concluídos localmente
Escopo: `api/`, `ui/`, `proto/`, `cdk/` e documentação operacional

## 1. Resultado esperado

Entregar uma camada social segura e persistente para o CTech Poker, com:

- amizade mútua;
- painel rápido de pessoas no lobby e página completa `/people`;
- jogadores recentes, limitados aos últimos 90 dias e 50 resultados por página;
- presença de amigos (`online`, `offline`, `in_table`) sem revelar a mesa privada;
- convites in-app para a mesa, sem mensagens diretas e sem push/e-mail;
- silenciar, bloquear e denunciar jogadores;
- remoção completa do fluxo de compra por Pix do pós-derrota;
- recuperação pós-derrota por recompra com saldo existente ou recompensa diária gratuita.

O matchmaking público não consulta bloqueios. Um jogador bloqueado ainda pode cair na mesma mesa pública e todas as
ações de poker continuam visíveis; somente chat, reações e interações sociais são suprimidos. Essa separação impede que
o bloqueio seja usado para evitar adversários fortes.

## 2. Decisões de produto fechadas

| Tema | Decisão |
|---|---|
| Relação | Amizade mútua, com solicitação, aceite e recusa |
| Descoberta | Código de amizade exato + ações em perfis/jogadores recentes; sem busca global difusa por nome |
| Limites | 200 amigos, 50 solicitações enviadas pendentes, 30 novas solicitações/dia |
| Presença | Visível apenas entre amigos; `in_table` não expõe `table_id` nem código de sala |
| Convite | Somente entre amigos; expira em 15 minutos; aceite não compra fichas nem ocupa assento |
| Notificações | Apenas in-app, persistidas por 90 dias |
| Mensagens diretas | Fora do escopo |
| Silenciar | Local ao observador, persistente entre dispositivos; oculta chat e reações |
| Bloquear | Inclui silenciar, desfaz amizade, cancela solicitações e impede novos convites sociais |
| Desbloquear | Remove o bloqueio, mas mantém o jogador silenciado até ação explícita de reativação |
| Matchmaking | Bloqueio não altera mesas públicas, sorteio de assentos ou ações do jogo |
| Denúncia | Persistida e revisável; não notifica nem remove automaticamente o denunciado |
| Pós-derrota | Pix removido do diálogo; loja continua existindo como rota deliberada e separada |
| Clubes/grupos persistentes | Fora desta entrega; a fundação de amizade poderá ser reutilizada depois |

## 3. Limites de domínio e invariantes

1. O jogador autenticado é sempre `claims.Sub`; nenhum endpoint aceita o ator da operação no corpo.
2. Leitura e escrita social são exclusivas do cliente first-party Poker com `sub` e `sid`. Não serão criados escopos
   públicos/delegáveis para lista de amigos, bloqueios, denúncias ou inbox.
3. O servidor nunca informa explicitamente que “o outro jogador bloqueou você”. Respostas usam um conflito genérico
   para evitar vazamento de estado privado.
4. Estados espelhados de amizade são alterados em uma única transação DynamoDB. Não pode existir `friend` em apenas um
   lado depois de uma resposta bem-sucedida.
5. Silenciar e bloquear nunca alteram `hand.State`, `Table.ViewFor`, resultado da mão, assentos ou matchmaking.
6. Conteúdo suprimido é filtrado antes de entrar no estado React: não cria bolha, animação, som nem anúncio ARIA.
7. Convite aceito para sala privada cria um grant temporário; o `share_code` nunca é enviado ao convidado.
8. Aceitar convite só autoriza a abertura da sala. Capacidade, moeda, termos, saldo e buy-in são revalidados no fluxo
   normal de `rooms/:id/join`.
9. Texto livre de denúncia e conteúdo de chat não entram em logs, métricas ou dimensões do CloudWatch.
10. O caminho de dinheiro real e o gate `REAL_MONEY_ENABLED` permanecem intactos.

## 4. Modelo de dados

### 4.1 `poker_social_edges`

Uma linha direcionada por observador e outro jogador:

```text
pk = <owner_player_id>
sk = <other_player_id>
relationship = outgoing | incoming | friend | ausente
muted = bool
blocked = bool
requested_at, friends_since, updated_at
```

O item pode continuar existindo sem `relationship` para guardar `muted` ou `blocked`. A amizade usa duas linhas
espelhadas. Solicitação de A para B grava `outgoing` em A e `incoming` em B na mesma transação.

Regras de transição:

- `none -> outgoing/incoming`: nova solicitação;
- solicitações cruzadas concorrentes convergem para `friend/friend`;
- `incoming/outgoing -> friend/friend`: aceite;
- recusa/cancelamento remove somente `relationship`, preservando flags locais;
- bloquear grava `blocked=true, muted=true` no lado do bloqueador e remove a relação nos dois lados;
- desbloquear grava apenas `blocked=false`; `muted` permanece;
- remover amizade limpa `relationship` nos dois lados, preservando flags locais.

Todas as transações usam condições sobre o estado anterior. Em conflito, o serviço relê as duas linhas e reaplica a
máquina de estados; não trata leitura prévia como mecanismo de correção.

### 4.2 `poker_recent_players`

Visão materializada atualizada ao final de cada mão:

```text
pk = <viewer_player_id>
sk = <opponent_player_id>
last_played_at = unix millis
last_table_id, last_hand_id, hands_together
gsi_recent_pk = <viewer_player_id>
gsi_recent_sk = <last_played_at>#<opponent_player_id>
ttl = last_played_at + 90 dias
```

Uma mão de nove jogadores produz até 72 pares direcionados, abaixo do limite de 100 itens de uma transação DynamoDB.
O mesmo `TransactWriteItems` inclui um guard `pk=hand#<table_id>#<hand_id>, sk=recent_guard`, com condição de ausência
e TTL de sete dias. Assim, retry/failover da conclusão da mão não incrementa `hands_together` duas vezes.
O hook pós-mão grava os pares depois do commit autoritativo e falha aberto: uma falha social nunca trava ou invalida a
mão. O endpoint filtra o próprio jogador e qualquer par bloqueado em qualquer direção.

Para não começar com a tela vazia após o deploy, o primeiro `GET /social/recent` sem itens faz bootstrap preguiçoso a
partir de até 100 mãos já existentes em `poker_player_hands`, somente para o usuário autenticado. Não há `Scan` global.

### 4.3 `poker_social_events`

Inbox durável para solicitações, aceite de amizade e convites:

```text
pk = <recipient_player_id>
sk = <event_id ULID>
type = friend_request | friend_accepted | table_invite
actor_id, status, room_id, created_at, expires_at
gsi_inbox_pk = <recipient_player_id>
gsi_inbox_sk = <created_at>#<event_id>
gsi_unread_pk = <recipient_player_id>#unread   # somente quando unread=true
gsi_unread_sk = <created_at>#<event_id>
ttl = created_at + 90 dias
```

Convites ganham `expires_at = created_at + 15 minutos`. Eventos expirados podem permanecer visíveis como histórico,
mas nunca voltam a conceder acesso. O GSI esparso de não lidos fornece contador e paginação sem varrer a partição.

### 4.4 `poker_player_reports`

Fila de moderação separada por retenção e acesso:

```text
pk = <target_player_id>
sk = <reporter_player_id>#<idempotency_key>
report_id, reporter_id, category, surface
table_id, hand_id, action_id, reaction_id
details, evidence_message, created_at
status = open | reviewing | resolved
resolution, resolved_at, resolved_by
gsi_status_pk = <status>
gsi_status_sk = <created_at>#<report_id>
ttl = ausente enquanto aberto; resolved_at + 180 dias quando resolvido
```

Categorias v1: `harassment`, `hate`, `spam`, `cheating`, `inappropriate_profile`, `other`. Superfícies v1:
`table_chat`, `table_reaction`, `table_behavior`, `profile`, `recent_player`.

Quando houver `action_id`, o servidor busca a ação em `poker_action_log`, verifica jogador/tabela/mão e copia a
evidência já sanitizada. `details` é opcional e limitado a 500 caracteres. O cliente não é fonte confiável para texto
de chat nem identidade do autor.

### 4.5 Alterações em tabelas existentes

- `poker_player_profiles`: adicionar `friend_code` e GSI esparso `gsi_friend_code`.
- `poker_rooms`: habilitar TTL e adicionar itens `sk=invite#<player_id>` para grants temporários de sala privada. O item
  `sk=meta` não recebe TTL.

O código de amizade será `PKR-XXXX-XXXX-XXXX`, derivado de SHA-256 do `user_id` e codificado em base32. É estável,
case-insensitive e possui 60 bits úteis. Perfis antigos recebem o código de forma preguiçosa em `GetOrCreate`; o GSI
esparso passa a indexá-los assim que voltarem ao produto. Uma consulta que encontrar colisão retorna indisponibilidade
e emite métrica, em vez de escolher uma conta arbitrária.

## 5. Contratos HTTP

Todas as mutações aceitam `Idempotency-Key`, validado e limitado a 128 caracteres. Eventos usam um ID determinístico
derivado de ator, alvo, operação e hash da chave; um retry retorna o resultado existente sem duplicar inbox. Operações
naturalmente indexadas por par continuam idempotentes. Adicionar `Idempotency-Key` ao CORS de `newFiberApp`.

| Método e rota | Resultado |
|---|---|
| `GET /v1.0/social/summary` | contadores + até 5 amigos online, 3 solicitações e 3 convites para o painel rápido |
| `GET /v1.0/social/friends?cursor=` | amigos paginados e presença atual |
| `GET /v1.0/social/requests?direction=incoming|outgoing&cursor=` | solicitações pendentes |
| `GET /v1.0/social/recent?cursor=` | jogadores recentes, mais recentes primeiro |
| `GET /v1.0/social/blocked?cursor=` | bloqueados pelo próprio usuário |
| `GET /v1.0/social/inbox?cursor=` | atividades e notificações |
| `POST /v1.0/social/inbox/read` | marca IDs conhecidos como lidos |
| `GET /v1.0/social/lookup/:friendCode` | resolve somente código exato e retorna perfil social mínimo |
| `GET /v1.0/social/relationships/:playerId` | estado do relacionamento para menus de mesa/perfil |
| `POST /v1.0/social/friend-requests` | envia por `target_player_id` ou `friend_code`, nunca ambos |
| `POST /v1.0/social/friend-requests/:playerId/accept` | aceita solicitação recebida |
| `POST /v1.0/social/friend-requests/:playerId/decline` | recusa recebida |
| `DELETE /v1.0/social/friend-requests/:playerId` | cancela solicitação enviada |
| `DELETE /v1.0/social/friends/:playerId` | desfaz amizade mutuamente |
| `PUT /v1.0/social/mutes/:playerId` | silencia localmente |
| `DELETE /v1.0/social/mutes/:playerId` | reativa chat/reações |
| `PUT /v1.0/social/blocks/:playerId` | bloqueia, silencia e desfaz relação |
| `DELETE /v1.0/social/blocks/:playerId` | desbloqueia sem reativar áudio/conteúdo |
| `POST /v1.0/social/table-invites` | envia convite a um amigo para a mesa atual |
| `POST /v1.0/social/table-invites/:eventId/accept` | valida e cria grant temporário; retorna sala sanitizada |
| `POST /v1.0/social/table-invites/:eventId/decline` | encerra convite |
| `POST /v1.0/social/reports` | cria denúncia idempotente e retorna `202` |

DTO social mínimo: `player_id`, `name`, `avatar_url`, `friend_code` somente quando apropriado, `relationship`,
`muted`, `blocked`, `presence`, `last_played_at`. Nunca retornar flags indicando “blocked_by_other”.

Erros funcionais usam Problem Details e códigos estáveis: `friend_limit_reached`, `request_limit_reached`,
`relationship_conflict`, `invite_expired`, `room_full`, `room_closed`, `report_rate_limited`. A mensagem pública de
`relationship_conflict` não revela se a causa foi bloqueio do destinatário.

## 6. Realtime e presença

### 6.1 Protobuf

Estender `proto/poker.proto` de forma aditiva:

- `PlayerPresence { player_id, status }`;
- `SocialEvent { event_id, type, actor_id, room_id, status, created_at, expires_at, presence }`;
- `ServerMessage.social_event` e `ServerMessage.unread_count` em novos field numbers;
- novos tipos de envelope: `social_event`, `social_presence_changed`, `social_inbox_count`.

Mutações continuam em HTTP; o socket é somente push/invalidação. Regenerar Go e TypeScript, nunca editar os arquivos
gerados manualmente.

### 6.2 Um único socket geral

`useLobbyRealtime` já consome `/v1.0/ws`. Ele será montado uma única vez em um bridge dentro de `QueryProvider`, com
conexão desabilitada quando não houver token. Remover as montagens duplicadas de `StakesGrid` e da loja. Não criar um
terceiro hook realtime.

Eventos sociais invalidam chaves específicas do TanStack Query e atualizam o contador da navegação. Reconexão invalida
`social/summary`, `social/inbox` e `social/friends`, porque deltas offline não são reproduzidos.

### 6.3 Presença efêmera no Valkey

Criar `internal/presence` sobre o cliente do `cache.RedisBackend`, mantendo um adaptador em memória apenas para dev e
testes. Não persistir presença em DynamoDB.

- um sorted set por jogador guarda connection IDs com score igual à expiração;
- open/heartbeat usa script atômico para remover conexões vencidas, registrar a atual e detectar transição `0 -> 1`;
- close remove a conexão e detecta `1 -> 0`;
- heartbeat de 30 s e expiração de 75 s toleram queda abrupta de instância;
- consultas de lista são pipelineadas e limitadas aos IDs da página;
- a sessão aberta mais recente reconcilia `in_table` no login; join/leave/remoção atualizam esse estado;
- apenas transições reais geram fan-out para `user#<friend_id>`.

O payload público para amigos contém só o status. Mesmo em `in_table`, não contém sala, blinds, saldo ou moeda.

## 7. Segurança na mesa

### 7.1 Filtragem de chat e reações

O backend continua persistindo e transmitindo a atividade pública da mesa para auditoria e compatibilidade. O frontend
carrega a lista autoritativa de IDs silenciados/bloqueados e filtra:

- `snapshot.chat_messages` antes de hidratar o histórico;
- frames `chat` antes de criar lista ou bolha;
- `snapshot.reactions` e frames `reaction` antes de criar animação;
- conteúdo já renderizado imediatamente quando mute/block é confirmado ou chega por realtime.

Assentos, apostas, nome, avatar, cartas reveladas e ações de poker nunca são filtrados. O envio do próprio chat/reação
permanece permitido mesmo que outro jogador tenha bloqueado o autor; a supressão pertence a cada observador.

### 7.2 Menu de jogador

Criar `PlayerActionsMenu` e substituir o gatilho isolado de nota no assento por um menu acessível, sem prejudicar o modo
de alvo das reações:

- ver perfil;
- adicionar amigo / solicitação pendente / remover amigo;
- editar nota privada;
- silenciar / reativar;
- bloquear / desbloquear, com confirmação;
- denunciar.

O mesmo núcleo de ações será reutilizado no perfil público e nas listas de `/people`. A ação otimista deve ser desfeita
em erro, mostrando a razão recuperável. Bloqueio remove imediatamente conteúdo do alvo da tela antes do round-trip e
faz rollback se o servidor rejeitar.

### 7.3 Denúncia e operação

Criar `api/cmd/moderation` com comandos `list`, `show`, `review` e `resolve`, usando o GSI de status. O comando exige
credenciais AWS operacionais; não é exposto pela API pública. `show` é a única operação que imprime texto livre.

Adicionar runbook com:

- triagem por categoria e severidade;
- bloqueio de avatar pelo mecanismo existente;
- encaminhamento manual de fraude/bot e suspensão de conta;
- resolução `no_action`, `content_removed`, `warning_requested`, `suspension_requested`;
- retenção e princípio de menor acesso.

Não haverá banimento ou remoção automática baseada apenas em volume de denúncias.

## 8. Experiência híbrida de Pessoas

### 8.1 Navegação e painel rápido

Adicionar `people` a `MainRoute` e à navegação de `AppPageChrome`. No lobby, um botão “Pessoas” abre drawer lateral em
desktop e sheet de altura controlada no mobile. Exibe:

1. solicitações pendentes;
2. convites de mesa;
3. amigos online;
4. jogadores recentes;
5. link “Ver todas as pessoas”.

O badge usa o contador de eventos não lidos e possui texto acessível; não depende apenas de cor.

### 8.2 Página `/people`

Rota estática exportável com tabs por query/local state: `Amigos`, `Solicitações`, `Recentes`, `Bloqueados` e
`Atividades`. Usar listas densas, não grade de cards. Cada tab possui:

- skeleton de carregamento;
- vazio específico e orientado à próxima ação;
- erro com retry;
- paginação por cursor;
- estados online/offline/em uma mesa;
- confirmação e feedback das mutações;
- cache útil durante perda temporária de conexão, marcado como possivelmente desatualizado.

O cabeçalho permite copiar o próprio código e resolver um código exato. Nome de exibição nunca é tratado como
identificador único.

### 8.3 Convite de mesa

O `InviteDialog` mantém copiar/compartilhar link e ganha seção “Amigos” com busca local na lista de amigos online. Ao
enviar, mostrar status por destinatário sem fechar o diálogo. O servidor exige:

- amizade ativa;
- ausência de bloqueio em ambos os sentidos;
- sessão aberta do remetente naquela mesa;
- sala existente e não arquivada;
- um convite pendente por par/sala;
- limites de 5 convites/minuto por destinatário e 20/minuto por remetente.

Ao aceitar, a UI abre `/table?id=<room_id>&social_invite=<event_id>`. Para sala privada, o grant em `poker_rooms`
autoriza o GET, WebSocket e join sem expor o share code. Sala lotada ou encerrada produz um estado final explicativo e
um CTA de volta ao lobby.

## 9. Pós-derrota sem Pix

Alterar `RebuyDialog` para usar `saldo disponível < buy_in_min`, e não somente `saldo == 0`.

Fluxos:

1. auto-rebuy bem-sucedido durante a janela existente: não abrir diálogo;
2. saldo suficiente: slider e recompra normal;
3. saldo sandbox insuficiente + recompensa disponível: botão “Resgatar fichas grátis”; após `spin`, invalidar perfil,
   recalcular saldo e habilitar recompra somente se atingir o mínimo;
4. recompensa em cooldown ou ainda insuficiente: explicar sem pressão e oferecer “Voltar ao lobby”;
5. dinheiro real insuficiente: não inserir compra; manter retorno ao lobby e a rota separada da carteira/loja.

Remover de `RebuyDialog` todas as importações, queries e estados de `listSkus`, `createPurchase`, `SkuGrid` e
`PixPaymentView`. A loja e seus endpoints continuam inalterados. Nenhum CTA do estado de derrota navega
automaticamente para compra paga.

## 10. Pacotes e arquivos previstos

### API

- Criar `internal/social/{model,store,service}.go` e testes.
- Criar `internal/recentplayers/{store,service}.go` e testes.
- Criar `internal/reports/{model,store,service}.go` e testes.
- Criar `internal/presence/{service,memory}.go` e testes de transição/expiração.
- Criar `internal/api/v1/social.go`, `social_test.go`, `reports.go`, `reports_test.go`.
- Estender `internal/api/v1/tablews.go` para presença e eventos sociais no socket geral.
- Estender `internal/api/v1/rooms.go` e `internal/roomstore` para grants de convite.
- Estender `internal/player` com `friend_code` e busca exata em GSI.
- Estender `internal/sessionlog` com consulta da sessão aberta mais recente.
- Estender `internal/tablestore` com leitura validada de ação por `action_id` para evidência.
- Estender `internal/config` e o carregamento SSM com `SOCIAL_GRAPH_ENABLED`; a flag controla amizade, presença e
  convites, mas não desliga mute, block ou report.
- Ligar stores/serviços em `internal/app/app.go` e rotas em `internal/api/v1/router.go`.
- Criar `cmd/moderation`.
- Atualizar `api/README.md`, `api/internal/oauthresource/scope-manifest.json` somente para descrever que social é
  first-party privado, e o runbook operacional.

### Protobuf

- Atualizar `proto/poker.proto`.
- Regenerar `api/internal/api/v1/proto` e `ui/src/lib/api/proto/poker.ts` pelo fluxo existente.

### UI

- Criar `app/people/{layout,page}.tsx` e testes.
- Criar `components/social/PeopleDrawer.tsx`, `PeopleList.tsx`, `FriendCodeLookup.tsx`, `SocialInbox.tsx`,
  `PlayerActionsMenu.tsx`, `ReportPlayerDialog.tsx` e testes.
- Criar `lib/api/social.ts` e `lib/social.ts` para contratos e seletores puros.
- Estender `AppPageChrome`, lobby, perfil público, `InviteDialog`, `Seat`, `TableStage` e página da mesa.
- Montar `useLobbyRealtime` uma vez no provider e estender seus testes.
- Estender `useTableRealtime` com supressão e testes de snapshot, frame ao vivo e mudança durante exibição.
- Refatorar `RebuyDialog` e seus testes.
- Atualizar mock runtime com os endpoints, eventos e cenários sociais.
- Atualizar `globals.css` com drawer/sheet/listas, foco, responsividade e `prefers-reduced-motion`.
- Atualizar `ui/README.md`, guia da comunidade e documentação de testes.

### CDK

- Adicionar as quatro tabelas e GSIs em `lib/dynamodb-stack.ts`.
- Adicionar GSI em perfis e TTL em salas.
- Threadar ARNs pelo `bin/poker.ts` e `lib/api-stack.ts`.
- Adicionar o parâmetro SSM `socialGraphEnabled` ao padrão já usado pelas flags operacionais.
- Incluir `BatchGetItem` para hidratação de perfis; manter recursos limitados às tabelas/indexes do Poker.
- Adicionar métricas sociais ao dashboard e alarmes de denúncias/rate limit.
- Atualizar testes de `dynamodb-stack` e `api-stack`, além de `cdk/README.md`.

## 11. Plano de execução por PR

### PR 1 — Infraestrutura e contrato wire

- criar tabelas/GSIs/TTL/IAM e seus testes;
- adicionar mensagens protobuf de forma compatível;
- introduzir os modelos e interfaces vazias dos novos pacotes;
- documentar os contratos.

Critério: `cdk synth` sem substituição destrutiva das tabelas existentes; clientes antigos ignoram os novos fields.

### PR 2 — Grafo social e código de amizade

- implementar máquina de estados transacional;
- geração/busca exata de código;
- endpoints de amigos, solicitações, mute e block;
- limites por usuário e IP;
- testes de corrida, idempotência e bloqueio cruzado.

Critério: nenhum teste consegue observar amizade unilateral após sucesso; bloqueio nunca altera APIs de sala pública.

### PR 3 — Inbox, convites e grants privados

- eventos duráveis e contador unread;
- envio/aceite/recusa de convite;
- grant temporário em `poker_rooms`;
- gates HTTP e WebSocket da sala privada;
- fan-out in-app pelo socket geral.

Critério: convidado entra em sala privada sem receber `share_code`; convite expirado/lotado falha antes do buy-in.

### PR 4 — Presença e jogadores recentes

- presença multi-instância em Valkey;
- reconciliação com sessão aberta;
- materialização pós-mão e bootstrap preguiçoso;
- hidratação em lote de perfis e filtro de bloqueios.

Critério: queda abrupta converte presença para offline em até 75 s; uma falha na escrita de recentes não afeta a mão.

### PR 5 — Denúncias e operação

- endpoint, validação de evidência e rate limit;
- CLI de moderação, métricas, alarmes e runbook;
- migrar denúncia de avatar para a fila nova, mantendo compatibilidade do estado legado.

Critério: texto livre não aparece em logs; retry com a mesma chave não cria segunda denúncia.

### PR 6 — UI Pessoas híbrida

- API client, mock runtime, realtime global único;
- drawer do lobby, badge e `/people` completo;
- código de amizade, solicitações, amigos, recentes, bloqueados e inbox;
- estados vazios/erro/offline/mobile/acessibilidade.

Critério: fluxo código -> solicitação -> aceite -> presença funciona em duas sessões e em viewport mobile.

### PR 7 — Segurança e convite dentro da mesa

- menu de ações nos assentos e perfil;
- filtro imediato de chat/reações;
- diálogo de denúncia;
- convite de amigos no `InviteDialog`.

Critério: jogador silenciado não gera histórico, bolha, animação ou anúncio; suas apostas e ações continuam visíveis.

### PR 8 — Pós-derrota e hardening final

- remover Pix embutido;
- integrar recompensa diária gratuita;
- métricas de funil, documentação final e QA visual;
- habilitação gradual do feature flag social.

Critério: nenhum caminho pós-derrota renderiza SKU, QR Code ou Pix; loja separada permanece funcional.

## 12. Estratégia de testes

### Go unitário

- tabela completa de transições do relacionamento;
- solicitação cruzada concorrente;
- aceite/recusa/cancelamento repetidos;
- limite de amigos e pendências;
- mute/block preservando flags locais;
- invite expirado, duplicado, bloqueado, sala privada e sala cheia;
- relatório com alvo próprio, categoria inválida, evidência divergente e retry;
- presença com múltiplas conexões, expiração e fechamento fora de ordem;
- recentes com 2, 6 e 9 jogadores, TTL e filtro de bloqueio.

### Go integração

- DynamoDB Local para transações espelhadas, GSIs e grants com TTL;
- duas goroutines disputando solicitação/aceite/block;
- duas instâncias simuladas com Valkey para presença e fan-out;
- fluxo sala privada: convite -> aceite -> WS -> join;
- mão completa materializa pares sem duplicar `hands_together` no retry.

### Frontend

- API modules, cursores e Problem Details;
- drawer e todas as tabs em loading/empty/error/success;
- optimistic update e rollback;
- evento realtime recebido após reconexão;
- filtro de chat/reação em snapshot e frame;
- mute aplicado enquanto conteúdo está animando;
- bloqueio mantém assento e ações de poker;
- convite expirado/lotado;
- `RebuyDialog` nos cinco fluxos descritos;
- navegação por teclado, foco do drawer/dialog e nomes acessíveis.

### E2E/manual

- duas contas: amizade completa, presença e convite;
- três contas: bloqueado reaparece na mesma mesa pública sem conteúdo social;
- dois dispositivos da mesma conta: mute e inbox sincronizam;
- desconexão forçada da API: presença expira e recupera;
- mobile 360x800 e desktop 1440x900;
- `prefers-reduced-motion`, zoom 200% e leitor de tela nos badges/diálogos.

### Quality gates

```text
api: go test ./... -race
ui: npx vitest run
ui: npx tsc --noEmit
ui: npx eslint src --max-warnings 0
ui: npm run build
cdk: npm test
cdk: npx cdk synth
repo: git diff --check
```

Não reduzir os thresholds de cobertura de 90% do frontend.

## 13. Observabilidade e privacidade

Métricas sem IDs de jogador, sala ou texto livre:

- `FriendRequestCreated`, `FriendRequestAccepted`, `FriendRemoved`;
- `TableInviteCreated`, `TableInviteAccepted`, `TableInviteExpired`;
- `PresenceTransitions`, `PresenceWriteFailures`;
- `PlayerMuted`, `PlayerBlocked`, `PlayerReported` por categoria enumerada;
- `RecentPlayersWriteFailures`;
- `SocialHTTPResponses` por template/status;
- `BustDialogShown`, `BustDailyRewardClaimed`, `BustRebuySucceeded`, `BustReturnedLobby`;
- `BustToStoreNavigation` medido apenas como navegação posterior, nunca como CTA do diálogo.

Alarmes:

- aumento de `PlayerReported` acima do baseline;
- qualquer erro sustentado de transação social;
- falha de presença acima de 1% em 5 minutos;
- rate limit social acima do baseline;
- backlog de denúncias abertas acompanhado pelo runbook, não por dimensão de usuário.

Logs estruturados usam `request_id`, operação e código de resultado. O único local autorizado a revelar `details` ou
`evidence_message` é `cmd/moderation show`, mediante acesso operacional explícito.

## 14. Rollout e rollback

1. Deploy de tabelas/GSIs/TTL e IAM.
2. Deploy da API com rotas aditivas e `SOCIAL_GRAPH_ENABLED=false`; safety/report pode permanecer ativo.
3. Smoke test em dev e sandbox com duas contas e duas instâncias.
4. Habilitar escrita de recentes por 24 h para aquecer dados e observar custo/erros.
5. Deploy do frontend com Pessoas oculto pelo flag.
6. Habilitar para equipe interna, depois 10%, 50% e 100% da audiência sandbox.
7. Manter dinheiro real fora do piloto social até concluir o hardening jurídico/operacional já pendente no projeto.

Rollback:

- desligar o flag remove amigos/presença/convites da UI sem apagar dados;
- safety continua disponível e não depende do painel social;
- mensagens protobuf novas são ignoradas por clientes antigos;
- grants expiram sozinhos;
- tabelas usam `RETAIN` em produção e não são removidas no rollback de aplicação.

## 15. Critérios finais de aceite

- amizade é sempre mútua e sincroniza entre dispositivos;
- nenhum endpoint social permite IDOR, M2M ou cliente third-party;
- código de amizade não permite busca difusa nem enumeração por nome;
- presença só é vista por amigos e não revela mesa privada;
- convite privado funciona sem compartilhar segredo da sala;
- bloqueio impede solicitação/convite e suprime chat/reações, mas não matchmaking;
- denúncia possui evidência confiável, fila revisável, retenção e runbook;
- jogadores recentes aparecem após uma mão e respeitam 90 dias/bloqueios;
- todos os estados vazios, concorrentes, expirados e offline possuem tratamento de UI;
- pós-derrota não contém Pix, QR Code, catálogo pago ou redirecionamento automático à loja;
- todos os quality gates passam e as documentações de API, UI, CDK e operação refletem o comportamento entregue.
