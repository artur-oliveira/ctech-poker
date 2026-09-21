# 2026-09-21 — quatro correções a partir da captura da mesa 01M327ZR25JS10AMJ7NWMK5YSE

Fonte: HAR de uma sessão de ~40 minutos (1.485 frames WebSocket na conexão da mesa, decodificados
contra `proto/poker.proto`). Quatro sintomas relatados pelo jogador, quatro causas distintas.

## 1. "Maior pote de hoje" parecia congelado

**Não era falta de atualização.** A captura mostra exatamente 4 leituras de
`GET /rooms/:id/highlights/today` em 40 minutos, e elas coincidem com o mount e com os 3 momentos
em que o recorde realmente mudou (154.250 → 221.250 → 224.350 → 395.350). O `invalidateAfterSettle`
com predicado `settled` funcionou como projetado: nenhuma leitura desperdiçada.

O que enganava o jogador era a métrica. A mão `01M328WD8SRSV25CCPV7BM7KS9` exibiu **206.750** no
feltro, mas só **81.000** foram disputados: um all-in de 144.750 pago por apenas 19.000 gerou uma
camada de refund de 125.750. O recorde de 154.250 sobreviveu, corretamente, contra um pote que o
jogador tinha acabado de ver maior na tela.

Correções:

- `internal/highlights.ContestedPot` extraído de `RecordHand` e documentado. Mantém a exclusão de
  refunds (um pote que ninguém contestou não pode liderar o dia) e passa a somar o `Amount` bruto da
  camada em vez do `PayoutAmount`. O segundo é líquido de rake, e rake não aparece no display do
  pote — gravá-lo deixava o troféu algumas centenas de fichas abaixo do número que a mesa inteira
  tinha acabado de ler.
- `ui/src/lib/tableOutcome.ts`'s `highlightPot` espelha exatamente a mesma conta (é o que permite ao
  cliente decidir se vale a pena reler).
- Rótulo passa a ser **"Maior pote disputado hoje"**, com `title` explicando que aposta não paga
  volta para quem apostou e não conta. Guia (`guide/table`) atualizado no mesmo tom.

## 2. "Ação inválida" instantânea — duas causas encadeadas

23 erros `stale_state` e 2 `invalid_action` na captura.

### 2a. Servidor: `version` exata como pré-condição de `act`

`validateActionPrecondition` exigia `expected_snapshot_version == a.version`. Mas `version` é o
token de concorrência otimista do item e é incrementado por **todo** commit, inclusive os que não
mudam nada visível: `peek_cards` (que por design *nunca* faz broadcast) e `reaction` (que só
publica o próprio frame dedicado). O cliente não tem como observar esses saltos, muito menos
ecoá-los. Resultado: qualquer jogador espiando as próprias cartas invalidava a ação legal de todo
mundo na mesa. Na captura, o cliente estava em v114, mandou `act` e levou `stale_state`; o
`sync_state` seguinte devolveu v116.

`StoredTable.GameplayVersion` (atributo `gameplay_version`, persistido no mesmo
`UpdateExpression` de `CommitAction`) passa a registrar a `version` do último commit **não
cosmético**. `tablestore.CosmeticAction` é a lista canônica (chat, reaction, peek_cards) e substitui
o antigo `cosmeticAction` local de `actor_commit.go`, que já servia para omitir o `ReplayFrame`.

`Actor.actionPreconditionHolds` aceita qualquer versão na janela
`[gameplayVersion, version]` com o mesmo `hand_id`. Toda propriedade de segurança do teste estrito
continua valendo: nada que tocou board, pote, rodada de aposta ou qualquer assento pode ter
acontecido no intervalo (isso teria movido `gameplayVersion`), e o motor segue rejeitando ação fora
de vez. É o mesmo raciocínio que `handlePreselect` já fazia via `expected_stage`.

Linhas escritas antes disso não têm o atributo; `0` é tratado como "igual a `version`", degradando
para o comportamento antigo em vez de aceitar uma ação arbitrariamente velha.

Cobertura: `internal/table/precondition_cosmetic_test.go` (unitário) e
`precondition_cosmetic_integration_test.go` (round-trip por DynamoDB e por uma **segunda instância**,
que é onde o drift realmente acontece em produção).

### 2b. Cliente: retry cego do `stale_state`

`stale_state` não é mostrado ao jogador — há retry automático. Mas o retry reenviava a ação contra a
versão nova **sem verificar se a vez ainda era do jogador**:

```
#892 v=638 cur=eu   → #893 act check(638) → #894 stale_state
#896 v=639 cur=OUTRO → #897 act check(639) → #898 invalid_action   ← alerta visível
```

O `check` já tinha sido resolvido pela própria preseleção do jogador (`call_any`, #887). O retry
disparava fora de vez e o servidor respondia "it is not player X's turn to act", que a UI traduz
como "Essa ação não é mais válida".

`retryTargetStillOpen` (`lib/hooks/useTableActionQueue.ts`) é a decisão pura que faltava: o retry só
acontece se o resync trouxer a mesma mão **e** o jogador ainda na vez. Caso contrário a ação
pendente é descartada em silêncio, que é o resultado correto — a vez já foi jogada.

## 3. Saída pendente travava o jogador e queimava o time bank

Linha do tempo: "Luvas" pede saída em v603; a vez chega a ele em v605 (t=…4994,9); **34 segundos de
silêncio**; v607 (t=…5028,9) já é o flop.

Duas metades.

### Servidor: o sweep de auto-fold parava no primeiro assento

`applyActAndCommit` retornava `applied && Stage() == Complete` num único `bool`. Ou seja: para todo
auto-fold de meio de mão o valor era `false`, e tanto `processPendingExitAutoFolds` quanto
`processInlinePreselections` liam isso como falha — abortavam o laço e forçavam um `ensureLoaded`
desnecessário. O laço nunca encadeava dois auto-folds: o segundo assento impossível de esperar ficava
no relógio até o turn timer, ou seja 15s + 30s de time bank que ninguém pediu e ninguém pode gastar.

A função agora devolve `(applied, completed, err)` e os dois sweeps testam `applied`.
Teste: `internal/table/pendingexit_sweep_test.go`.

### Cliente: o botão de cancelar sumia justamente quando era necessário

`ExitStatus` escondia **Cancelar saída** sob `isViewerTurn`, apoiado num invariante comentado no
próprio arquivo: "once it's their turn the fold is already committed". Isso só vale quando o
`request_exit` chega com o jogador **já** no relógio — `hand.RequestExit` deliberadamente não o
folda em nenhum outro caso, delegando ao sweep. Com o sweep atrasado, o jogador ficava sem botões de
ação (a saída pendente os remove) e sem saída: exatamente o "impossibilitado de jogar" relatado.

O cancelamento agora fica disponível enquanto a saída estiver pendente, inclusive na própria vez.
`hand.CancelExit` já é bem-definido nesse caso e nunca desfaz um fold já dado.

## 4. Resumo da sessão nunca aparecia

`useTableRemoval` escrevia `['seated'] = {seated:false}` no mesmo efeito que criava o `sessionRecap`.
O gate `if (!seated) return <BuyInPanel/>` de `app/(app)/table/page.tsx` roda **antes** de onde o
`<SessionRecap/>` era renderizado, então a mesa inteira desmontava e a tela de rebuy tomava o lugar
do resumo no mesmo commit. O `queueMicrotask(() => setSessionRecap(...))` era uma tentativa de
ordenar os updates que não resolve nada: o gate vence de qualquer forma.

O recap passa a ter seu próprio branch, **antes** do gate de `!seated` — necessário também porque a
essa altura o socket já foi derrubado e `rt.snapshot` não existe mais. O assento só é realmente
liberado quando o jogador fecha o resumo (`closeRecap`, que já fazia isso).

Os testes existentes de `useTableRemoval` cobriam só o hook isolado, com `setQueryData` mockado e
inerte — por isso o bug passou. A regressão agora é coberta na suíte de integração da página.

## Ainda aberto

A metade servidor do item 3 explica por que o sweep não encadeia, mas **não** explica por que o
primeiro fold de "Luvas" não saiu em v605. O frame v607 traz `pending_exit=false` e `ready=true`
para ele, o que só `CancelExit` produz, e o deadline de v607 é o de v605 acrescido de 65ms — assinatura
de divergência entre instâncias (ver `2026-09-17-table-snapshot-divergence-and-highlight-winner.md`).
Fechar isso exige `action_log` + `table_state_history` da mesa `01M327ZR25JS10AMJ7NWMK5YSE` nas
versões 603–609.
