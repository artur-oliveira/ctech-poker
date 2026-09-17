# Spec — Divergência de snapshot entre instâncias e vencedor do highlight

**Data:** 2026-09-17
**Evidência:** `bug_poker.har` (mesa `01M2JSJHB4KD1H0TZY65SG5WYX`, 2026-09-15 15:15–15:21 UTC),
465 frames WebSocket decodificados com `protoc --decode=poker.ServerMessage -I proto proto/poker.proto`.

---

## 1. Sintomas relatados

1. O card "Maior pote de hoje" ficou sem o nome do vencedor depois de um all-in vencido.
2. O badge de streak alternava entre `v1` e `d1` repetidas vezes.
3. Cartas do board às vezes não renderizavam por completo; apareciam alguns segundos depois.

## 2. Evidência

### 2.1 Highlight sem vencedor

Correlação de 3/3 entre `won_without_showdown` e a ausência de `revealed` no highlight:

| hand_id | pote | `won_without_showdown` | `revealed` |
|---|---|---|---|
| `…6T5BW9` | 50.250 | true | ausente |
| `…J5ZCCC` | 56.250 | false | presente |
| `…8XYEXT` | 150.875 | true | ausente |

Frame final de `…8XYEXT`: `stage: complete`, `winners: 78fd4d57…`, `payouts: {78fd4d57…: 150875}`,
`hand_category: three_of_a_kind`, `won_without_showdown: true`, e
`hole_cards_revealed: [false, false]` em todos os assentos.

### 2.2 Frames duplicados com overlays divergentes

O mesmo `(hand_id, snapshot_version)` chega múltiplas vezes na mesma conexão
(histograma de frames por versão: 3× em 73 versões, 4× em 10, 5× em 8, 8× em 1).
Diff campo a campo dos 107 grupos duplicados — **só dois campos divergem**:

```
current_streak: 256 divergências
equity:         208 divergências
```

Exemplo (v170, 30 ms entre frames, byte-idênticos fora isso):

```
msg 48: Dexther:2  Matheos:-3  Artur:-2  Kelizinha:-4
msg 49: Dexther:1  Matheos:-2  Artur:-1  Kelizinha:-3   ← uma mão atrasado
msg 51: Dexther:2  Matheos:-3  Artur:-2  Kelizinha:-4
```

Equity do próprio viewer na mesma versão: `0.25 → 0.19 → 0.25`, `0.0725 → 0.0625 → 0.0725`
(90 versões afetadas).

### 2.3 Entrega de cartas

Zero regressões de versão, zero board inconsistente entre frames da mesma versão, zero board
encolhendo. Cada street chega num único frame com o board completo. Nenhum erro de rede.

## 3. Causa raiz

### 3.1 Highlight (`api/internal/highlights/store.go`)

`Highlight` guarda `Board` e `Revealed`, e **nada** sobre quem ganhou. `revealedHandsOf` só copia
participantes com `PlayerHandInfo.Revealed == true` — vazio quando não há showdown.
A UI (`ui/src/components/table/TodayHighlight.tsx`) deriva o nome exclusivamente de `revealed`,
reavaliando as mãos no cliente. Sem showdown → sem nome.

O mesmo componente já documenta um segundo defeito: o caption nomeia *a melhor mão bruta entre as
reveladas*, não quem levou o maior pote — errado em side pot.

### 3.2 Broadcast duplicado (`api/internal/table/actor_presence.go`, `actor_views.go`)

`api/internal/app/app.go:570` publica em `reg.Broadcast(ctx, tableID+"#"+viewerID, …)`.
`ws.RedisRegistry.Broadcast` (api-commons v1.9.2) faz `PUBLISH` num canal Valkey; cada instância
mantém uma subscrição e entrega às suas conexões locais. **A entrega é fleet-wide: um único publish
de qualquer instância alcança o socket do jogador onde quer que ele esteja.**

`handleExternalChange` documenta o oposto — *"re-broadcasts to whichever of this table's players are
connected to THIS process"* — e chama `broadcastAll()`. Como o publish não é local, cada instância
que roda um Actor dessa mesa reemite o mesmo snapshot versionado ao mesmo cliente.

Sobre esse snapshot, duas decorações são calculadas **no momento do broadcast**, fora do estado
versionado, com cache por instância:

- `applyStreaks` (`actor_views.go:394`) lê `a.streaks`, preenchido por `refreshStreaks` com
  `StreakRefreshInterval = 30s`. Só a instância que vence `claimHandHooks` chama
  `SetStreaksForActor`; as demais servem o valor da mão anterior por até 30 s.
  Janelas medidas no HAR: 15:15:52→58, 15:18:08→26, 15:21:12→33.
- `equityFor` (`actor_views.go:440`) → `equity.EstimateForTableWithStats`, cujo Monte-Carlo usa
  `rng := rng64{state: rand.Uint64()}` (`equity.go:227`) — semente aleatória por processo, logo
  cada instância produz um número diferente para a mesma entrada.

`ui/src/lib/tableSnapshotReducer.ts:21` rejeita versão **estritamente** menor
(`if (version < latestVersion) return null`), então frames de versão igual são aceitos e
sobrescrevem o estado: last-writer-wins → o badge e a equity piscam.

> **`snapshot_version` não é sequência de broadcast.** `a.version` só incrementa em `commit`
> (`actor_commit.go:127,159`); chat, reactions e presença reemitem na mesma versão. Deduplicar por
> versão no cliente (`<=`) quebraria chat e reactions. A correção pertence ao servidor.

### 3.3 Cartas incompletas

Sem causa raiz provada. O servidor está inocentado pela evidência em §2.3. O HAR está filtrado (sem
assets), então uma falha no fetch da face SVG não pode ser confirmada nem descartada.
Candidato mais forte, ligado a §3.2: cada versão custava 3 decodes de protobuf (~150 KB) + 3
re-renders completos da mesa, e `docs/2026-09-10-card-reveal-visibility.md` já documenta que a
animação de entrada fica `pending` (com `fill-mode: both` no keyframe `opacity: 0`) quando o decode
do snapshot cai no mesmo frame em que o flip monta.

## 4. Requisitos

**R1.** O highlight persiste o vencedor real da mão (id, nome e payout), independente de showdown.

**R2.** A UI nomeia o vencedor a partir de R1; a categoria da mão só é anexada quando esse vencedor
também está em `revealed`. Rows escritas antes do deploy (sem o campo novo) continuam renderizando
pelo caminho atual.

**R3.** Um commit de uma instância produz **um** frame por viewer, não um por instância.

**R4.** Dois frames com o mesmo `(hand_id, snapshot_version)` para o mesmo viewer são idênticos,
qualquer que seja a instância que os emitiu.

**R5.** O badge de streak do vencedor chega ao cliente no fim da mão, sem esperar o próximo commit.

**R6.** Nenhuma leitura Valkey adicional por comando (a regressão que `StreakRefreshInterval`
resolveu em #222). Leituras extras são aceitáveis por mão.

## 5. Fora de escopo

- Mover streak/equity para dentro do estado persistido e versionado (`tablestore`). É a correção
  estrutural definitiva, mas muda schema; R3+R4 entregam o comportamento observável sem isso.
- Correção especulativa do sintoma 3. Depois de R3 a carga de render cai ~3×; a revalidação precisa
  de um HAR sem filtro ou de um performance trace.

## 6. Nota cross-repo

O erro de premissa "registry fleet-wide + comentário assumindo entrega process-local" está no uso de
`gopkg.aoctech.app/api-commons/ws`, compartilhado por toda a família CTech. Vale auditar
ctech-account, ctech-wallet, ctech-billing e ctech-dfe pelo mesmo padrão de reemissão duplicada.
