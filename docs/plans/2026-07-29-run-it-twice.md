# Plano de Implementação — Run It Twice

Data: 2026-07-29 · Escopo: `api/`, `ui/` · Spec: `docs/specs/2026-07-28-player-avatars-and-next-features.md` (Feature 2)

## 📌 Contexto

Em all-in com ação encerrada, o board restante é distribuído **duas vezes**, cada metade valendo metade
do pote. Reduz variância sem mudar EV.

O motor já tem a peça central: o runout pausado. `IsAwaitingRunoutForActor()` (`hand.go:1208`) detecta
"stage Flop/Turn, rodada completa, `remaining > 1 && canStillAct <= 1"`, e
`AdvanceRunoutStreetForActor()` (`hand.go:1182`) entrega **uma** street por chamada, sem rodada de
aposta, chamando `runShowdown()` ao alcançar o river (`:1194-1196`). O ator arma o passo seguinte com
`armRunoutTimer` (`actor.go:1529`, `RunoutStreetDelay = 2600ms` em `turntimeout.go:39`) e comita
`ActionLogEntry{Action: "runout_step"}` (`actor.go:1564`).

Ou seja: o loop de "distribuir board sem apostas, com pacing" está construído e testado
(`runout_test.go`). Este plano o roda duas vezes.

## 📌 Nota de Arquitetura: o provably-fair sai de graça

As duas runouts saem do **mesmo baralho já comprometido**. `Table.nextCard` (`hand.go:107`) é um cursor
simples e `dealCard()` (`hand.go:968`) só incrementa; a segunda runout consome as posições seguintes.

Duas consequências, ambas verificadas:

1. **`RootCommitHash` já prova as duas.** É computado sobre todas as 52 cartas
   (`deck/deck.go:154`) e publicado no snapshot desde `StartHand` (`snapshot.go:277-281`).
   **Nenhuma primitiva criptográfica nova.**
2. **A prova por posição já cobre as cartas extras sem mudança.** `fairnessProofFor`
   (`snapshot.go:301`) revela salt+carta para toda posição `i < t.nextCard` (`snapshot.go:357`). Como a
   segunda runout avança `nextCard`, ela entra na prova automaticamente.

O gate de revelação de seed continua `!hasUnrevealedFold` (`snapshot.go:315-317`). RIT acontece em
all-in, que normalmente tem foldados, então a maioria das mãos RIT cai no caminho **sem seed**, com
prova por posição. Isso é o comportamento correto e já implementado — **não** afrouxar o gate para
"melhorar" a prova de RIT. Ampliar a revelação de seed expõe hole cards mucked.

### Correção à spec: não há conflito com rabbit hunt

A spec avisa "cuidado: interage com rabbit hunt". Verificado: não interage. Rabbit hunt
(`snapshot.go:326-343`) só age quando `wonWithoutShowdown && len(t.board) < 5`; RIT só existe em
showdown. São mutuamente exclusivos pela condição, não por regra nova.

O que **exigiria** cuidado se mudasse: rabbit hunt calcula índices aritmeticamente da ordem de deal
(`holeTotal := len(t.handOrder)*2`, flop em `holeTotal+1`, turn em `+5`, river em `+7` —
`snapshot.go:320-321`). RIT não muda a posição de nenhuma carta da **primeira** runout, só consome
depois. A aritmética fica intacta. Um teste deve travar isso.

### Nota: consentimento pré-declarado, não prompt por mão

RIT no ecossistema costuma ser prompt por mão. Aqui é **preferência pré-declarada**, resolvida no
momento do all-in.

Por quê: um prompt por mão exige um timer novo, um deadline, e uma decisão sobre desconexão durante o
prompt — tudo isso *dentro* de um caminho que hoje é pausa determinística por 2600 ms. Uma pausa
interativa nesse ponto é a coisa mais fácil de travar a mesa que existe no motor.

Pré-declarado cobre o desejo real ("quero menos variância") com um `bool` por jogador por mesa,
resolvido quando o all-in fecha: **todos os jogadores ainda na mão precisam ter optado**, senão roda
uma vez. Sem stall possível.

**Consequência aceita:** não dá para pedir RIT só no pote grande. Se isso virar pedido recorrente, o
prompt por mão é uma feature separada — e aí o deadline vira requisito explícito, não detalhe.

---

## Fase 1 — Motor

### T1 — Opção de sala

`roomstore.Room` (`room.go:6-33`) ganha `RunItTwiceEnabled bool` com tag
`dynamodbav:"run_it_twice_enabled"`, ao lado de `EquityDisplayEnabled:24` — que é o único flag booleano
de gameplay hoje e é exatamente o padrão a copiar.

Caminho completo, todos os cinco pontos que `EquityDisplayEnabled` toca:

1. `roomdto.go:13` — `CreateRoomRequest.RunItTwiceEnabled *bool` (tri-state: ausente ≠ false).
2. `rooms.go:103-110` — default. **Default `false`**, ao contrário de equity (que default `true`): RIT
   muda a distribuição de resultados e não deve aparecer sem alguém pedir.
3. `rooms.go:115-129` — struct literal.
4. `tablews.go:958` — `ConvertRoom`. `Room` no proto: **próximo número livre é 18**.
5. `tablews.go:327,347` — `actor.SetRunItTwiceEnabledForActor(room.RunItTwiceEnabled)`, no formato de
   `SetEquityEnabledForActor`.

**Não** restringir a salas privadas. As restrições de `rooms.go:80-82,93-95` existem para escalada de
blind e timeout, que dão vantagem ao criador; RIT é simétrico.

### T2 — Consentimento por jogador

Preferência por jogador **na mesa**, não no perfil: quem quer RIT no Mid pode não querer no Micro.

1. `poker.proto` `ClientMessage` — tipo `"set_run_it_twice"` com `optional bool run_it_twice`.
   **Próximo número livre no `ClientMessage` é 15.** O comentário de tipos em `:167` também precisa da
   entrada nova (hoje ele já está desatualizado: `"bot_challenge"` é tratado e não está listado).
2. `commands.go` — `SetRunItTwiceCmd{PlayerID string, Enabled bool, Reply chan error}`, no formato de
   `ShowCardsCmd:100-107` (que é o precedente de decisão não-aposta por jogador).
3. `tablews.go:472` — case no switch, no formato de `"show_cards":570-579`.
4. `actor.go:200-247` — dispatch + `handleSetRunItTwice`, no formato de `handleShowCards:1187`
   (engine call → `commit` com `ActionLogEntry` → `broadcastAll`). O log é o que faz a preferência
   sobreviver a troca de instância.
5. `hand.go:49-84` — `Player.RunItTwice bool` com `dynamodbav:"run_it_twice,omitempty"`, e um
   `SetPlayerRunItTwiceForActor(playerID string, enabled bool) (changed bool)` no formato de
   `SetPlayerNameForActor:288-299` — o retorno `bool` evita commit no-op, que `actor.go:440` já usa.
6. `SeatView` (`snapshot.go:107-129`) expõe `RunItTwice bool` **só para o próprio viewer**. `ViewFor`
   (`snapshot.go:190`) já é a única fronteira de visibilidade; a preferência de RIT de um oponente é
   informação de tendência, então segue a mesma regra das hole cards.

### T3 — Decisão de rodar duas vezes

Uma função pura, avaliada quando o runout pausado começa:

```go
// shouldRunItTwice decide no momento em que a ação fecha, uma vez por mão.
// Requer: flag da sala, exatamente 2+ jogadores ainda na mão, e TODOS eles com
// a preferência ligada. Discordância roda uma vez — a alternativa (maioria)
// impõe variância a quem não pediu.
func (t *Table) shouldRunItTwice() bool
```

Gravado em `Table.runItTwice bool` quando `IsAwaitingRunoutForActor()` (`hand.go:1208`) fica true pela
primeira vez na mão, e limpo em `StartHand`. **Decidido uma vez**: reavaliar por street permitiria que
uma desconexão no meio do runout mudasse a regra com metade do board na mesa.

Persistir em `State` (`state.go`) ao lado de `NextCard:28` — o runout atravessa commits.

### T4 — A segunda runout

`AdvanceRunoutStreetForActor()` (`hand.go:1182`) hoje: completa a street que falta, e chama
`runShowdown()` no river (`:1194-1196`).

Passa a ter duas fases. `Table` ganha `runoutPhase int` (1 ou 2) e `boardTwo []deck.Card`:

```
fase 1: como hoje, mas ao alcançar o river NÃO chama runShowdown se runItTwice;
        em vez disso guarda o board completo, restaura o board ao ponto do all-in
        (o prefixo comum), e entra na fase 2
fase 2: completa de novo a partir do prefixo comum, usando dealBoardCard/dealFlop
        (que continuam avançando nextCard); ao alcançar o river, runShowdown
```

O **prefixo comum** é o board que existia quando a ação fechou — as cartas já viradas não são
redistribuídas. Guardar `runoutSplitAt int` (`len(t.board)` no momento da decisão) e derivar as duas
runouts dele. Isso evita duplicar estado de board: `t.board` é a runout 1 completa, `t.boardTwo` só
carrega as cartas *depois* de `runoutSplitAt`.

Usar `dealFlop()`/`dealBoardCard()` (`hand.go:976,981`) na segunda runout também — **com burn**. Duas
razões: é a convenção do resto do ecossistema, e é o que mantém a prova por posição descrevendo o
mesmo baralho sem caso especial.

**Suficiência do baralho, a verificar em teste:** pior caso 9 jogadores all-in pré-flop = 18 hole + 3
burns + 5 board = 26 posições na runout 1, mais 3 burns + 5 = 34 no total. Cabe em 52 com folga, mas o
teste é o que garante que ninguém mexa no deal order sem notar.

`armRunoutTimer` (`actor.go:1529`) é idempotente por `handID+stage` (`:1530-1544`). Com duas fases, a
chave passa a ser `handID+phase+stage`, senão a fase 2 não arma no mesmo stage já visto pela fase 1.
**Este é o bug mais provável de toda a feature.**

Pacing: 2600 ms por street × até 4 streets = ~10,4 s do all-in ao showdown. Aceitável para all-in, mas
vale considerar reduzir `RunoutStreetDelay` na fase 2 — a tensão já passou. Deixado como decisão de
produto, não incluído.

### T5 — Divisão do pote

Aqui está a decisão que mais fácil se erra.

**Calcular os side pots UMA vez**, com `ComputeSidePots` (`sidepots.go:31`) sobre as contribuições
reais, e então dividir **cada `PotLayer.Amount` pela metade**. A alternativa óbvia — halvar as
contribuições e rodar `ComputeSidePots` duas vezes — produz contribuições fracionárias e quebra as
invariantes do pacote.

**A rake é aplicada UMA vez**, na camada cheia, antes da divisão. `runShowdown` aplica rake em
`hand.go:1338-1341` dentro do loop de camadas; rodar o loop duas vezes cobraria rake duas vezes. Este é
o erro que custa dinheiro do jogador e não aparece em teste de "quem ganhou".

Ordem em `runShowdown` (`hand.go:1242`):

```
1. ComputeSidePots(contribuições)          — uma vez
2. rake por camada                          — uma vez  (hand.go:1338-1341)
3. para cada camada:
     half := (amount - rake) / 2
     odd  := (amount - rake) % 2
     avalia vencedores da camada no board 1 -> paga half + odd
     avalia vencedores da camada no board 2 -> paga half
4. credita as stacks                        (hand.go:1370-1382, um crédito por jogador)
5. bust -> SittingOut, payouts, stage=Complete, rotateDealer  (:1383-1388, :1462-1463)
```

**A ficha ímpar da divisão pela metade vai para a runout 1.** Regra determinística e documentada; não é
a mesma pergunta que a ficha ímpar *entre vencedores empatados*, que continua resolvida por
`oddChipWinner` (`hand.go:1466`, varre `handOrder` do dealer+1) dentro de cada avaliação.

Os caminhos de refund (`hand.go:1265-1280` camada com um único elegível, `:1307-1337` todos foldaram)
**não** dividem: refund não é pote disputado. Sair cedo desses branches antes de halvar.

### T6 — `HandOutcome` e persistência

`HandOutcome` (`hand.go:147-189`) ganha:

```go
BoardTwo []string `json:"board_two,omitempty"` // vazio = rodou uma vez
```

Manter `Board` como a runout 1 preserva **todos** os consumidores atuais sem mudança — é o que evita
que esta feature toque em histórico, replay e share ao mesmo tempo.

`PotResult` (`hand.go:215`) ganha `Runout int` (1 ou 2; 0 = mão sem RIT), para o histórico conseguir
explicar por que um jogador recebeu duas parcelas. `Payouts` continua agregado (soma das duas), que é o
que a UI precisa para stack.

Persistência:
- `sessionlog.HandItem` (`store.go:51-77`) ganha `board_two`; populado em `app.go:~310-382`.
- `tablestore.ReplayFrame` (`store.go:82-93`) ganha `BoardTwo []string` ao lado de `Board`.
- `Snapshot` (`snapshot.go:18`) ganha `BoardTwo []string`, ao lado de `RunoutCards:40`.
- `poker.proto` `TableSnapshot` — `repeated string board_two = 33` (**33 é o próximo livre**,
  confirmado). Conversão em `tablews.go:899-932`; `ProtocolVersion` em `tablews.go:924` sobe para 10.

## Fase 2 — Frontend

### T7 — Dois boards

`Board.tsx` tem 28 linhas e renderiza `cards.map` com placeholders vazios até 5 (`:23-26`). Aceita
`boardTwo?: string[]` e, quando presente, renderiza a segunda fileira abaixo.

**As cartas comuns aparecem uma vez**, no topo, e as duas fileiras mostram só as cartas divergentes —
repetir o flop duas vezes gasta espaço horizontal que a mesa não tem e sugere que o flop foi
redistribuído. O `runoutSplitAt` do servidor é o que diz onde cortar; enviar como `board_split_at` no
snapshot em vez de o cliente inferir.

Pacing de reveal: as animações já são por índice (`--deal-index`, `globals.css:2600-2602`, 780 ms com
stagger de 320 ms; `PlayingCard.tsx:18`; `Board.tsx:23-24` passa `index={index < 3 ? index : 0}` e
`slow={index === 4}`). A segunda fileira usa os mesmos índices para o stagger — não inventar
constantes novas.

`HandReplayer.tsx` monta snapshot sintético e reusa `TableStage` (`:101,132`), com as constantes
espelhadas em `:26-29`. Passar `board_two` do frame; nada mais muda.

### T8 — Preferência na UI

`TablePreferencesDialog.tsx` é o lugar (já hospeda tema de feltro, voz do dealer, reality check). Toggle
que dispara `setRunItTwice` do `useTableRealtime`, no formato de `showCards`/`keepSeat`
(`useTableRealtime.ts:666-723`).

O toggle só aparece quando a sala tem `run_it_twice_enabled`, e o texto tem que dizer que **todos** os
envolvidos precisam ter ligado — senão o jogador liga, roda uma vez, e reporta como bug.

`CreateRoomDialog.tsx` ganha o checkbox de criação (schema zod em `:26-30`).

## Fase 3 — Testes

Go (estilo do pacote: stdlib `testing`, sem testify; `runout_test.go` já dirige mesa real com
`NewTable` + `StartHand` + `Act` e acessa campos não exportados):

1. **All-in heads-up com ambos optando roda duas vezes**: dois boards de 5, prefixo comum idêntico,
   sufixos diferentes.
2. **Um dos dois não optou → roda uma vez.** `BoardTwo` vazio.
3. **Flag da sala desligada → roda uma vez** mesmo com todos optando.
4. **Metades somam o pote.** `sum(Payouts) + rake == sum(Contributions)`. É a invariante que pega erro
   de arredondamento e rake duplicada de uma vez.
5. **Rake cobrada uma vez.** Comparar com a mesma mão sem RIT: rake idêntica. Este teste é o que impede
   o erro caro.
6. **Ficha ímpar da metade vai para a runout 1**, deterministicamente.
7. **Camada com um único elegível não divide** (refund, `hand.go:1265-1280`).
8. **Todos foldaram → refund, sem RIT** (`hand.go:1307-1337`).
9. **9 jogadores all-in pré-flop não estouram o baralho.** `nextCard <= 52` no fim.
10. **`armRunoutTimer` arma na fase 2 no mesmo stage já visto na fase 1.** Chave `handID+phase+stage`.
11. **Prova de fairness cobre as duas runouts**: toda posição `< nextCard` tem salt+carta em
    `fairnessProofFor`, e `VerifyPartial` (`deck.go:164`) passa nas duas.
12. **Seed não é revelado quando há fold não revelado**, com RIT ligado. Regressão de segurança.
13. **Aritmética de rabbit hunt intacta**: mão sem RIT que termina em fold produz exatamente os mesmos
    índices revelados de hoje.
14. **Recuperação no meio do runout**: `ExportState`/`NewTableFromState` (`state.go`) no meio da fase 2
    retoma no lugar certo.

UI (vitest, thresholds de `vitest.config.ts` — 80/80/80/70):

15. `Board` sem `board_two` renderiza idêntico ao de hoje (snapshot).
16. `Board` com `board_two` mostra o prefixo comum uma vez.
17. Toggle ausente quando a sala não tem a flag.

## 📊 Resultado esperado

| Antes                                   | Depois                                            |
|-----------------------------------------|---------------------------------------------------|
| All-in resolve em um board              | Dois boards quando todos os envolvidos optam      |
| Variância cheia em all-in               | Metade do pote por runout                         |
| `RootCommitHash` prova um runout        | Prova os dois, sem cripto nova                    |
| Runout pausado tem uma fase             | Duas fases, timer com chave `handID+phase+stage`  |

## 🔮 Fora deste plano

- **Prompt de consentimento por mão.** Exigiria deadline, tratamento de desconexão durante o prompt e
  risco de travar a mesa no ponto mais delicado do motor. Pré-declarado cobre o caso de uso.
- **Run it three times.** A divisão em terços reabre arredondamento e não tem demanda.
- **`RunoutStreetDelay` reduzido na fase 2.** Provavelmente melhor UX; decisão de produto.
- **Estatística de RIT** (quantas vezes salvou/custou). Interessante e barato depois, mas exige campo
  novo em `pokerstats` por mão.
- **RIT em mesa de dinheiro real.** Não muda a base legal (a taxa é de reserva, não do pote), mas a
  divisão do pote em duas avaliações merece uma passada de conferência antes de ligar em modo real.
