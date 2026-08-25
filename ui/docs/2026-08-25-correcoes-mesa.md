# Correções da mesa — 25 de agosto de 2026

Três defeitos relatados por jogadores, investigados a partir do console de um cliente e do HAR
`player.har` (raiz do repositório). Os chunks minificados citados no relato existem em
`out/_next/static/chunks/`, o que permitiu localizar a função exata sem source maps.

## 1. Queda na mesa: `TypeError: y.some is not a function`

**Sintoma:** a mesa caía para o error boundary (`SystemState` 500, “Não conseguimos concluir esta jogada.”)
e reentrava em laço. O console mostrava o erro repetido em pares, vindo de
`0dqx5ptmjoka0.js` (reporter de erro do Next) e `3yr02ric1_rmh.js` (o `error.tsx` do app) — ou seja,
os dois eram apenas quem registrava, não quem quebrava.

**Causa raiz:** colisão de `queryKey`. A chave `['wallet', 'reaction-purchases']` era usada por dois
tipos de query diferentes:

- `src/app/store/page.tsx` — `useInfiniteQuery`, cujo cache guarda `{pages, pageParams}`;
- `src/app/table/page.tsx` — `useQuery`, que espera `ReactionPurchase[]`.

O TanStack Query indexa o cache apenas pelo hash da chave; ele não distingue uma query paginada de
uma simples. Quem passava pela loja e entrava na mesa dentro do `gcTime` recebia o objeto da loja
como `purchases`. O default `= []` não protege, porque só vale para `undefined`. Em
`TableReactions.premiumState` a chamada `purchases.some(...)` estourava — confirmado no bundle:

```js
function I(e){...:y.some(a=>a.reaction_id===e&&"refunding"===a.status)?"unavailable":...}
```

`premiumState` é chamada dentro do `.map` do painel de reações, exatamente como o stack indicava
(`at I` → callback anônimo → `Array.map`).

**Por que piorava com a rede oscilando:** o refetch da mesa normalmente sobrescreveria o cache com o
array correto em seguida. Com a API inacessível o refetch nunca conclui, então a forma errada
permanecia e a tela ficava travada no laço de erro.

**Correção:** chaves distintas e nomeadas em `src/lib/api/reactionPurchases.ts`
(`REACTION_PURCHASE_HISTORY_KEY` para a loja, `REACTION_PURCHASE_FIRST_PAGE_KEY` para a mesa). Ambas
continuam sob o prefixo `['wallet']`, então a invalidação única de `WALLET_QUERY_ROOT` segue valendo.
Teste de regressão em `reactionPurchases.test.ts`: semeia o cache com a forma infinita e exige que a
chave da mesa continue vazia.

**O erro de CORS no relato não é a causa.** `Cross-Origin Request Blocked … /v1.0/health` com
`Status code: (null)` é como o navegador expõe uma conexão recusada/derrubada (o mesmo cliente
depois mostrou `net::ERR_CONNECTION_CLOSED`): a API não respondeu, então nem os cabeçalhos CORS
existiam. O laço de `setTimeout` que aparece na pilha é o `NetworkProvider` sondando `/v1.0/health`
com backoff (teto de 30 s, `livenessPollDelay`) — comportamento projetado, não defeito de cliente.

## 2. Rabbit hunt nunca aparecia

**Causa raiz:** impasse entre cliente e servidor. Em `api/internal/engine/hand/snapshot.go`, numa mão
ganha sem showdown o servidor só publica `shuffle_server_seed_hex`, `revealed_card_salts`,
`unrevealed_card_hashes` e `runout_cards` **depois** que o jogador pagou (`t.rabbitHuntPaid[viewerID]`).
`RabbitHunt.tsx` exigia justamente esses campos para exibir o botão que dispara o pagamento — o
botão dependia do dado que só o pagamento produz.

Confirmado nos frames da mesa em `player.har`: todo snapshot `stage=complete`,
`won_without_showdown=true`, `board=4` chega com `shuffle_commit_hash` e `root_commit_hash`
presentes e `shuffle_server_seed_hex`/`runout_cards` vazios.

**Correção:** a oferta passa a ser condicionada ao **compromisso** do baralho
(`shuffle_commit_hash` ou `root_commit_hash`), não à prova. A verificação continua no navegador,
sobre o snapshot pago, sem nenhuma flexibilização — o invariante de justiça não muda.

## 3. Desktop sem os botões de preferências e convite

O redesenho móvel concentrou as ações num menu “⋮” (`TableUtilityMenu`) e passou
`showTrigger={false}` para `TablePreferencesDialog` e `InviteDialog`. Só que
`.table-utility-menu-slot` é `display: none` fora de retrato ≤1023 px: no desktop as duas ações
ficaram sem nenhuma porta de entrada.

**Correção:** os gatilhos próprios voltaram (`showTrigger` no padrão). As faixas
`.table-preferences-standalone` e `.table-invite-standalone` já eram ocultadas em retrato, então o
celular continua com o menu “⋮” e sem duplicidade.

## 4. Diagnóstico que saiu daqui para a API

Os mesmos HARs explicaram dois relatos que não são do cliente. Ambos têm a mesma raiz: **várias instâncias da
API servem a mesma mesa** (por projeto — `api/internal/tablemanager`, o lease é só afinidade de cache) e todas
transmitem para os mesmos sockets, então qualquer estado guardado só na memória de um processo aparece em
duas versões para o mesmo assento. Corrigido em `api/` (ver `api/README.md`, seção “Game-server model”):

- **Selo de sequência alternando V2/V4.** Decodificando o `player2.har`, o `snapshot_version` sobe de forma
  monótona e o streak alterna entre dois conjuntos coerentes — inclusive dois frames com `v=101` e valores
  diferentes. `Actor.streaks` era um mapa por processo, alimentado só pelas mãos que aquele processo rodou.
  Agora vive no Valkey (`api/internal/tablestreak`).
- **Jogadores expulsos sem motivo.** O kick por desconexão armava um timer de 5 min a partir de uma marca em
  memória; quem caía numa instância e reconectava por outra deixava a marca da primeira sem limpar — ela nunca
  vê o reconnect — e era removido e sacado no meio da sessão. `handleKickTimeout` agora exige evidência
  persistida (`LastActionAt`) antes de remover.
