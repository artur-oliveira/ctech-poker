# Runbook — moderação social do CTech Poker

## Objetivo e acesso

Este runbook cobre a fila `poker_player_reports`. Ela não executa banimento, remoção de conteúdo ou suspensão
automaticamente: volume de denúncias é sinal para triagem humana, nunca prova suficiente.

Use uma identidade AWS operacional individual com `dynamodb:GetItem`, `dynamodb:Query` e `dynamodb:UpdateItem`
limitados à tabela `<env>_poker_player_reports` e ao índice `gsi_status`. Não use a role da aplicação e não compartilhe
credenciais. `details` e `evidence_message` são dados restritos; somente o comando `show` pode imprimi-los.

```bash
ENVIRONMENT=prod AWS_REGION=us-east-1 go run ./cmd/moderation list --status open --limit 50
ENVIRONMENT=prod AWS_REGION=us-east-1 go run ./cmd/moderation show --target PLAYER_ID --key STORAGE_KEY
ENVIRONMENT=prod AWS_REGION=us-east-1 go run ./cmd/moderation review --target PLAYER_ID --key STORAGE_KEY --moderator OPERADOR
ENVIRONMENT=prod AWS_REGION=us-east-1 go run ./cmd/moderation resolve --target PLAYER_ID --key STORAGE_KEY --moderator OPERADOR --resolution no_action
```

Nunca cole a saída de `show` em tickets, chat ou logs sem aplicar a política interna de dados pessoais. `list`, `review`
e `resolve` não imprimem texto livre.

## Triagem

1. Liste `open` do mais antigo para o mais recente e mova o item para `reviewing` antes de investigar.
2. Use `show` somente quando categoria, superfície e referências estruturadas não forem suficientes.
3. Priorize ameaça imediata, ódio direcionado e assédio persistente; depois fraude/cheating, spam e perfil inadequado.
4. Para `table_chat` e `table_reaction`, confirme que a evidência copiada corresponde ao alvo. Ela veio do action log
   autoritativo e já passou pela sanitização do servidor; não substitua pelo texto fornecido pelo denunciante.
5. Para `table_behavior`/`cheating`, encaminhe manualmente ao fluxo de fraude/bot com tabela, mão e ação. Não conclua
   cheating apenas por resultado, estilo de jogo ou volume de denúncias.
6. Para avatar inadequado confirmado, use o mecanismo operacional existente de bloqueio de avatar e só então resolva
   como `content_removed`.

## Resolução e escalonamento

Resoluções permitidas:

- `no_action`: evidência insuficiente, conteúdo permitido ou duplicidade sem nova ação;
- `content_removed`: avatar/conteúdo removido pelo mecanismo operacional apropriado;
- `warning_requested`: solicitar advertência pelo canal autorizado de suporte/conta;
- `suspension_requested`: encaminhar suspensão à equipe de contas/risco. O Poker não suspende automaticamente.

Pedidos de advertência ou suspensão precisam de ticket operacional separado com acesso restrito. Registre no ticket o
`report_id`, nunca o texto livre. Depois da resolução, a linha recebe TTL de 180 dias; itens abertos ou em revisão não
expiram.

## Monitoramento e incidentes

O dashboard de operações, os alarmes e as métricas EMF `PlayerReported`/`SocialRateLimited` foram removidos em
2026-08-19 — não há mais namespace `CtechPoker/<env>`. O sinal agora é log estruturado em `/ctech-poker/<env>/app`,
consultado por Logs Insights; nada avisa sozinho. Verifique o backlog com `list --status open`. Se voltar a instrumentar,
mantenha a regra: nunca crie dimensão por jogador, sala, mão ou denúncia.

Se a criação de denúncias falhar de forma sustentada, confirme saúde/permissões da tabela e do GSI, preserve os logs
estruturados sem adicionar corpo HTTP e escale para on-call. Não execute Scan global e não reduza os controles de acesso
para acelerar a triagem.
