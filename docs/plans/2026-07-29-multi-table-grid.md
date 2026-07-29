# Plano de Implementação — Grid multi-mesa (2–4 mesas)

Data: 2026-07-29 · Escopo: `ui/` · Spec: `docs/specs/2026-07-28-player-avatars-and-next-features.md` (Feature 4)

## 📌 Contexto

Jogar 2–4 mesas simultâneas numa grade, com foco automático na mesa onde é a vez do jogador.

## 📌 Nota de Arquitetura: o backend não muda — nem um pouco

A spec diz que "o backend praticamente não muda" e que o gateway de lobby (`GET /v1.0/ws`, canal
`user#<id>`) "já é o lugar certo para o sinal de é sua vez na mesa X". Verificado: **a afirmação é
verdadeira e mais forte do que ela sugere, mas o mecanismo proposto está errado.**

- A string `user#<id>` não aparece em `ui/src`; é canal server-side. `useLobbyRealtime`
  (`useLobbyRealtime.ts:28-63`) trata só `error`, `room_created`, `room_updated`, `payment_received`,
  `system_broadcast`. **Não existe sinal de "é sua vez".**
- Não precisa existir. Com 4 mesas abertas, **cada mesa já tem seu próprio socket**, e cada um já
  entrega o sinal: `useTableRealtime.ts:114-116` dispara `playSound('your_turn')` quando
  `next.current_player_id === viewerId`. O foco automático lê `snapshot.current_player_id` de cada
  instância. **Zero mudança de backend, zero mensagem nova, zero endpoint.**

Rotear pelo lobby seria *pior*: um socket a mais, um caminho de sinal a mais, e informação que a mesa já
tem em mão.

### O hook já é multi-instância seguro

Auditado: todo estado de `useTableRealtime` é `useState`/`useRef` no corpo do hook (`:151-224`). Nenhum
singleton de módulo dentro. `activeTableIDRef` (`:153-159`) + `receiveForTable` (`:464-466`) já gateiam
mensagem por mesa, e `snapshot` só sai se `snapshotTableID === id` (`:668`).

Os singletons de módulo que existem são compartilhados **corretamente**:

- token de acesso (`client.ts:21`) — read-only, `Set` de listeners (`:33-38`), N assinantes é o desenho.
- `refreshInFlight` (`session.ts:16`) — colapsa N `recoverSession()` numa única renovação. **Isso é o
  que impede 4 sockets de dispararem 4 refreshes.** Custo de token não multiplica.
- preferências de mesa (`tablePreferences.ts:54-70`) — global por decisão, e continua certo.

Custo que **multiplica**: o heartbeat de 20 s por socket
(`@aoctech/ws-client/dist/heartbeat.js:8-9`) — 4 pings a cada 20 s, irrelevante. E
`MAX_RECONNECT_ATTEMPTS = 10` com desistência permanente (`useWebSocket.js:136-137`) por socket, que
com 4 mesas é 4× mais chance de uma delas morrer sem volta.

**Conclusão: o hook não é o trabalho. O trabalho é CSS, teclado, IDs de DOM e chrome fixo.** É por isso
que este é o plano mais longo dos cinco apesar de não ter backend.

---

## Fase 1 — Os bloqueadores reais

### T1 — `.game-table` não encolhe

```
2466 .game-table { position: relative; width: min(920px, 80vw); ... }
```

`80vw` é relativo à **viewport**, não ao pai. Numa célula de grade de 50% de largura, a mesa continua
ocupando 80% da tela e transborda. Não é um ajuste, é o bloqueador número um.

Trocar por `width: min(920px, 100%)` e deixar a célula ditar a largura. `.game-rail:2474` e
`.game-felt:2483` já são `inset` percentual, então acompanham de graça.

O shell da página também assume que é dono da viewport:

```
1620 .game { min-height: 100dvh; display: grid; grid-template-rows: auto minmax(320px,1fr) auto }
```

O `100dvh` e as três linhas (chrome / stage / action bar) passam a valer para o **grid**, não para cada
painel. O painel vira `height: 100%` dentro da célula.

Posicionamento de assento é percentual sobre `.game-table` (`globals.css:3451-3494`, `.seat-0`…
`.seat-8`) — acompanha. Mas `.game-seat:2665-2677` tem `padding: 7px 10px` e `border-radius: 14px`
fixos: o cartão de assento **não escala**. Em célula de 1/4 de tela os assentos vão ficar
proporcionalmente enormes. Precisa de um `--seat-scale` na célula, ou de aceitar que o grid usa o layout
compacto (ver T2).

### T2 — `TableStage` escolhe layout pela viewport, não pela célula

`TableStage.tsx:19` usa `VERTICAL_STAGE_QUERY = '(orientation: portrait) and (max-width: 1023px)'` via
`window.matchMedia` (`:22,30`). Numa grade, a orientação da viewport não diz nada sobre a forma da
célula: desktop landscape com 4 células dá 4 células retrato.

**Não há container query em nenhum lugar de `globals.css`** (lista completa de media queries auditada:
`957`…`7677`, todas viewport) e **não há `ResizeObserver` em `src` não-teste** (só o mock em
`test/setup.ts:26-35`).

Duas saídas:

1. **`TableStage` aceita `layout?: 'auto' | 'oval' | 'vertical'`.** O grid passa explicitamente; a
   página de mesa única continua `'auto'` e não muda nada. Mínimo, previsível, testável.
2. Container queries de verdade (`container-type: inline-size` na célula). Mais correto, mas introduz um
   mecanismo de layout novo num CSS de 7715 linhas que hoje não usa nenhum.

**Recomendado: (1).** O grid sabe quantas células tem; ele não precisa medir para decidir.

Consumidores de `TableStage` são só dois (`table/page.tsx:428`, `HandReplayer.tsx:132`), então a prop
nova é barata.

### T3 — Teclado: dois listeners `window`, sem noção de foco

`ActionBar.tsx:295-311` (`f`/`c`/`p`) e `:168-207` (`r`, `h`, `a`, setas) registram em **`window`**. O
único guard é `unavailable` (`:277` = `!connected || !isTurn || pending !== null || executing…`) e
`inactive` (`:169`).

Com dois `ActionBar` montados e vez em **duas** mesas ao mesmo tempo — que é justamente o cenário que
multi-mesa cria — apertar `f` foldaria as duas. Em dinheiro real isso é perda direta.

Correção: `ActionBar` ganha `keyboardEnabled?: boolean` (default `true`, então a página de mesa única não
muda). O grid liga em **exatamente uma** célula, a focada. O `if (unavailable) return undefined` de
`:296` passa a ser `if (unavailable || !keyboardEnabled)`.

Não construir contexto de foco de teclado: uma prop booleana que o grid controla é o mesmo resultado com
uma fração do código.

### T4 — IDs de DOM duplicados

Montar `ActionBar` duas vezes duplica `id`s literais, e `id` duplicado **quebra** `htmlFor` e
`aria-describedby` — o navegador resolve para o primeiro. É regressão de acessibilidade, não estética:

| id | arquivo | referenciado por |
|---|---|---|
| `raise-amount` | `ActionBar.tsx:224` | `htmlFor` do label, `aria-describedby` `:224` |
| `raise-amount-output` | `ActionBar.tsx:230` | — |
| `action-context` | `ActionBar.tsx:315` | `aria-describedby` em `:224,233,330,333,336` |
| `last-winners-panel` | `LastWinners.tsx:49` | — |
| `private-player-note` | `PlayerNoteDialog.tsx:87` | — |
| `table-theme-label`, `reality-check-label` | `TablePreferencesDialog.tsx:42,74` | `aria-labelledby` |

Correção: `useId()` do React em cada um, com o valor propagado para as referências. Padrão nativo, sem
prop nova. Faz sentido mesmo sem multi-mesa — e por isso pode ser um commit isolado, antes do resto.

### T5 — Chrome `position: fixed` colide

Nove elementos são `position: fixed` e se sobrepõem como 4 cópias:
`.mock-controls:2302`, `.reconnect-notice:2380`, `.idle-warning:2398`, `.game-chat:3809`
(`right:16px; bottom:var(--action-bar-clear)`), `.table-reactions:3935` (`right:70px`),
`.table-reaction-layer:4073`, `.last-winners:4386` (`left:16px`), `.achievement-toast:4499`,
`.api-notifier:4565`.

Três destinos distintos, e a distinção importa:

- **Sobem para o shell do grid, uma instância só**: `.achievement-toast`, `.api-notifier`,
  `.mock-controls`. São globais por natureza.
- **Ficam, mas só na célula focada**: `.game-chat`, `.table-reactions`, `.last-winners`. Chat de 4 mesas
  simultâneas é ilegível; o painel segue o foco.
- **Passam a ser por célula, `absolute` dentro do painel**: `.reconnect-notice`, `.idle-warning`. São o
  caso em que *qual* mesa importa mais que a mensagem — um "reconectando" sem dizer qual mesa é pior que
  nenhum aviso.

## Fase 2 — O grid

### T6 — Extrair `TablePane`

`app/table/page.tsx` tem 500 linhas e lê a mesa de query param (`:141`, `params.get('id')`,
validado pelo regex `ROOM_ID:50`) — não de segmento de rota. `TableContent` já é o componente interno
(`:491-500` embrulha em `TermsGate` + `Suspense`).

Extrair `TablePane({tableId, focused, onRequestFocus})` de `TableContent`, recebendo `tableId` por
**prop** em vez de ler `useSearchParams`. `/table` passa a ser um wrapper de uma linha que lê o param e
monta um `TablePane focused`. Nenhuma mudança de comportamento na mesa única — e é o que permite testar
o painel sem mockar `next/navigation`.

Estado que já é per-mount e continua correto por painel: `tableOpenedAt:148`, `nextHandArmed:201`,
`previousPayoutsRef:207`, `rememberedStart:209`, `scopedHandOutcome:211`, `activeTablePanel:213`,
`noteOpponent:180`. Vários já se comparam com `tableID` (`:222,232,340`) porque a navegação por query
param reusa um mount — esse escopo fica redundante mas inofensivo.

Chaves de query: `['room',id]`, `['seated',id]`, `['hands',id]` já são por mesa (`:151,162,171`).
`['sessions','me']:175` e `['player-notes']:178` são globais — **corretamente**, uma busca serve as 4.

### T7 — A rota

`/tables?ids=a,b,c` — plural, e `ids` para não colidir com o `id` singular de `/table`.

Limite duro de 4, validado na leitura do param. Acima de 4 a experiência degrada e a pressão de decisão
passa a favorecer bot — é uma decisão de integridade, não de performance.

Layout: `grid-template-columns: repeat(2, 1fr)` para 2–4 mesas, `1fr` para 1. Sem
`grid-template-areas`, sem breakpoints novos além dos que já existem.

### T8 — Foco automático

Uma mesa está "pedindo ação" quando `snapshot.current_player_id === viewerId`. O grid mantém
`focusedId` e o move quando uma mesa não focada começa a pedir ação **e a focada não está pedindo** — o
contrário roubaria o foco no meio de uma decisão, que é a forma mais rápida de fazer alguém pagar um
raise por acidente.

Foco manual (clique na célula) fixa por alguns segundos antes de a automação voltar a agir. Sem isso,
clicar numa mesa para ler o histórico é inútil.

`PerimeterTimer` (28 linhas, `PerimeterTimer.tsx:3-15`) é SVG puro sem timer nem `Date.now()` — CSS
`animationDuration`/`animationDelay` inline (`:20-21`) com `restartKey` (`:17`). **4 instâncias não
custam nada.** O problema é hierarquia visual, não recurso: a célula focada mostra o anel cheio, as
outras mostram uma versão reduzida. Puramente CSS por `[data-focused]`.

### T9 — Som e notificação

Três problemas verificados, cada um com correção diferente:

1. **`playSound` cria um `new Audio()` por chamada** (`sound.ts`), sem pooling nem volume. 4 mesas
   revelando board = 4 áudios sobrepostos. Correção mínima: no grid, só a **mesa focada** toca som de
   board/aposta; `your_turn` toca de qualquer mesa (é o sinal que importa) mas com dedupe de ~300 ms
   para não empilhar.
2. **`notify.ts:57-66` dedupa mensagens idênticas em 600 ms.** Quatro mesas com a mesma mensagem viram
   um toast — perde informação exatamente quando há mais dela. Correção: incluir o `tableId` no texto
   ou na chave de dedupe quando o grid está ativo.
3. **`useDealerVoice.ts:11-13` chama `window.speechSynthesis.cancel()`** antes de falar e no cleanup.
   `speechSynthesis` é singleton do navegador: 4 instâncias se cancelam mutuamente, e o resultado é voz
   cortada aleatoriamente. Correção: no grid, só a célula focada instancia `useDealerVoice`. Não há
   correção "compartilhada" barata aqui — a API não tem fila por origem.

### T10 — `RealityCheck` conta tempo errado no grid

`RealityCheck.tsx:53-62` conta de `joinedAt` **por mesa**, com supressão por `isTurn || open` (`:54`) da
própria instância. Quatro mesas = quatro diálogos, cada um contando a própria sentada.

Isso é substantivamente errado para jogo responsável: uma sessão de 4 mesas por 1 hora é mais exposição
que uma mesa por 1 hora, e o desenho atual mostra o aviso *mais* vezes com texto *menor* ("30 min nesta
mesa"), o que subestima o total.

Correção: **uma** instância no shell do grid, contando do `joinedAt` mais antigo entre as sessões
abertas, e somando mãos das 4. `intervalMs` continua vindo de `tablePreferences.ts:25` (0/30/60/90/120,
default 60 em `:23`). Supressão passa a ser "alguma mesa está pedindo ação", não "esta mesa".

### T11 — `ActiveTableBanner` já assume uma mesa

`ActiveTableBanner.tsx:10-15` faz `sessions.find(s => s.ended_at === 0)` e linka para a **primeira**
sessão aberta — mas o endpoint pode devolver várias. É um bug latente hoje, que multi-mesa transforma em
bug visível: o banner apontaria para uma mesa arbitrária das 4.

Passa a listar as sessões abertas e a linkar para `/tables?ids=...` quando houver mais de uma.

## Fase 3 — Testes

vitest, thresholds de `ui/vitest.config.ts` (lines/functions/statements 80, branches 70), glob
`src/**/*.test.{ts,tsx}`. Padrão de mock a copiar: `useTableRealtime.test.tsx:5-29` (mocka
`@aoctech/ws-client` capturando `options` e expondo `onMessage`/`onOpen`) e `app/table/page.test.tsx`
(mocka `next/navigation`, react-query, `useTableRealtime`, e captura props em `mocks.stageProps`).

1. **Dois `TablePane` abrem dois sockets independentes**, com URLs distintas, e uma mensagem de uma mesa
   não altera o snapshot da outra. É a garantia que o `activeTableIDRef` gate realmente isola.
2. **`keyboardEnabled: false` não reage a `f`/`c`/`p`/`r`/`h`/`a`.** O teste central de T3.
3. **Vez em duas mesas simultâneas + `f`**: exatamente **uma** ação enviada, na mesa focada. É o cenário
   de perda de dinheiro.
4. **`useId` produz ids distintos** em dois `ActionBar`, e `aria-describedby` de cada um resolve para o
   próprio `action-context`.
5. **Foco não é roubado** quando a mesa focada está pedindo ação.
6. **Foco move** quando a focada não pede e outra começa a pedir.
7. **Foco manual sobrevive** ao próximo sinal automático dentro da janela.
8. **`ids` com 5 mesas é rejeitado**; com 0 ou id inválido cai no mesmo tratamento de `ROOM_ID:50`.
9. **Uma instância de `RealityCheck`** no grid, contando do `joinedAt` mais antigo.
10. **Som de board só da mesa focada**; `your_turn` de qualquer mesa.
11. **`TableStage layout="vertical"` não consulta `matchMedia`** — a prop ganha do media query.
12. **`/table?id=x` renderiza igual ao de hoje** depois da extração de `TablePane` (snapshot de
    regressão; é o que protege a mesa única de virar dano colateral).

Manual, porque nenhum teste de jsdom pega: 4 mesas em desktop 1080p, conferir que nenhum painel
transborda e que os assentos são legíveis; memória e contagem de sockets no DevTools depois de 30 min.

## 📊 Resultado esperado

| Antes                                            | Depois                                          |
|--------------------------------------------------|-------------------------------------------------|
| Uma mesa por vez, id em query param              | 2–4 painéis, `?ids=`, mesa única inalterada     |
| `f` dispara em qualquer `ActionBar` montado      | Só na célula focada                             |
| `id`s de DOM literais, duplicáveis               | `useId`, `aria-describedby` correto por painel  |
| 9 elementos `fixed` colidiriam                   | Globais no shell, contextuais na célula focada   |
| `RealityCheck` conta por mesa, subestima o total  | Uma instância, tempo agregado                   |
| `.game-table` a `80vw` transborda em célula      | `min(920px, 100%)`                              |

## 🔮 Fora deste plano

- **Mais de 4 mesas.** Limite de integridade, não técnico.
- **Container queries.** Prop `layout` explícita resolve; container query seria o primeiro do repo.
- **Escalar `.game-seat`** proporcionalmente à célula. Se o layout compacto não bastar, é um `--seat-scale`
  numa passada separada — mexer em `padding`/`border-radius` de assento afeta a mesa única.
- **Layout salvo / arranjo customizado de painéis.** Grade 2×2 fixa primeiro.
- **Backend.** Nada. Explicitamente nada — nenhuma mensagem, nenhum endpoint, nenhum canal.
- **Reordenar painéis por urgência.** Mesa pulando de posição sob a mão do jogador é pior que a grade
  estática.
