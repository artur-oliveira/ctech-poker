# ADR: Session handoff between devices (#353)

## Contexto

`internal/tableconn.Service` mantém, por mesa, um conjunto fleet-wide
`playerID -> expiry` em Valkey — display state only, nunca usado para
decisão de auto-kick (essa decisão depende só de `LastActionAt`
persistido). `internal/table` (`Actor`) já suporta múltiplas conexões
concorrentes por jogador via `ConnectCmd`/`DisconnectCmd` +
`activeConns map[playerID]map[connID]struct{}`, mas `activeConns` só
enxerga sockets terminando neste processo — qualquer instância pode
rodar o `Actor` de qualquer mesa, e hoje não existe nenhum jeito de uma
instância fechar deliberadamente um socket que outra instância
segura.

Handoff explícito ("continuar aqui, desconectar o outro dispositivo")
precisa de duas coisas que não existem: saber quais `connID`s de um
jogador estão vivos em qualquer lugar da frota, e um jeito de fechar um
`connID` específico onde quer que ele esteja.

## Decisão

### 1. `tableconn` passa a rastrear conexão, não só jogador

`map[playerID]expiry` → `map[playerID]map[connID]expiry`. Mesmo
`EntryTTL`/`SyncInterval`/`KeyTTL`, mesmo read-modify-write. `Sync`
passa a receber `map[playerID][]connID` locais em vez de
`[]playerID`, e devolve `map[playerID]map[string]bool` (connID vivo).
Continua puramente display + agora também é a fonte de verdade fraca
("fraca" no mesmo sentido que hoje: perder uma escrita concorrente
custa um intervalo de dado errado) de "quais connIDs deste jogador
existem na frota agora" — é isso que o handoff precisa para saber o
que fechar, já que `Actor.activeConns` é local ao processo.

Callers existentes de `Sync` (`ActiveConnCount`-adjacent code em
`actor.go`) passam a enviar `connID`s reais em vez de só presença
binária — já os têm, vêm de `ConnectCmd`.

### 2. Novo comando de actor: `RequestHandoffCmd`

```go
type RequestHandoffCmd struct {
    PlayerID  string
    NewConnID string
    Reply     chan error
}
```

Dispatch: lê o snapshot fleet-wide do `tableconn` para `PlayerID`,
monta a lista de `connID`s a fechar (todo `connID` do jogador exceto
`NewConnID`). Se a lista for vazia (só a própria conexão nova, ou
`tableconn` indisponível/nil), é um no-op — não é erro, só não havia o
que assumir.

### 3. Fechamento remoto via canal Pub/Sub dedicado

Novo pacote `internal/tablehandoff`, mesmo padrão de
`internal/tablenotify` (que hoje só sinaliza "algo mudou" sem payload)
mas carregando dado: `Service.RequestClose(ctx, tableID string,
connIDs []string)` publica `{tableID, connIDs}` (JSON) num canal
Valkey `poker:handoff:<tableID>` usando o `valkey.Client` dedicado a
realtime (o mesmo que `tablenotify`/`ws.RedisRegistry` já usam — nunca
o client de cache genérico, pelo motivo já documentado no
`api/CLAUDE.md` sobre head-of-line blocking).

Cada instância assina uma vez por processo
(`tablemanager.Manager.ListenForHandoffCloses`, paralelo a
`ListenForExternalChanges`) e, ao receber `{tableID, connIDs}`, chama
`wsdrain.CloseByConnID(connIDs)` — nova função em `wsdrain` que fecha
(1001, mesmo `WriteControl` que `CloseAll` já usa) qualquer `connID`
que essa instância reconheça e ignora silenciosamente os que não são
dela. `wsdrain` precisa passar a indexar por `connID`, não só por
`Conn` — `Track`/`Untrack` ganham uma variante `TrackByID(connID
string, c Conn)` chamada de `tablews.go` junto com `reg.Register`.

Fire-and-forget, igual a `tablenotify`: um `RequestClose` perdido só
significa que o dispositivo antigo continua vivo até o próximo
`Sync`/TTL — nunca uma inconsistência de estado de mesa, porque nada
aqui decide estado, só fecha socket.

### 4. Frame de fechamento é sempre um close real do servidor

Não é "avisa o cliente antigo e espera ele se desconectar sozinho".
`wsdrain.CloseByConnID` escreve o controle 1001 diretamente — mesma
garantia que já existe pro shutdown gracioso. Satisfaz o critério de
aceite "nunca é silencioso" sem depender do JS do cliente antigo
cooperar (podia estar travado, código antigo, etc).

### 5. Ordem com ação em voo — não precisa de lógica nova

`Actor.Run` processa um comando por vez, uma goroutine por mesa.
`RequestHandoffCmd` é só mais um comando na mailbox: qualquer comando
da conexão antiga já enfileirado à frente dele comita normalmente
antes do handoff ser processado; nenhum comando novo dela chega depois
porque, uma vez que o socket fecha, o read loop dela morre. Isso é o
mesmo raciocínio que já garante `ConnectCmd`/`DisconnectCmd` sem
corrida — não é uma garantia nova, é reaproveitar a existente.
Resultado: nunca perde nem duplica uma ação, sem lock adicional.

O único caso a testar explicitamente: `RequestHandoffCmd` chega
**entre** o commit de uma ação da conexão antiga e o servidor
terminar de escrever a resposta pro socket dela — nesse caso a
resposta pode falhar ao escrever num socket já fechado. Isso já é
tratado hoje (erro de write num socket morto é ignorado, o
`DisconnectCmd` natural do read-loop morto faz a limpeza) e não muda.

## Consequências

- `tableconn`'s formato de dado no Valkey muda; não há migração —
  chaves existentes decodificam como mapa vazio na próxima leitura
  (mesma tolerância a "esqueceu tudo" que já existe pra qualquer TTL
  expirado).
- Novo canal Pub/Sub (`poker:handoff:<tableID>`) e nova função em
  `wsdrain` — ambos seguem o padrão de pacotes existentes, nenhuma
  abstração nova.
- `Actor.activeConns` não muda de forma; continua local por design.

## Critérios de aceite (do #353) — como são satisfeitos

- Handoff nunca é silencioso: `wsdrain.CloseByConnID` sempre escreve
  1001, nunca deixa expirar por TTL.
- `tableconn` rastreia conexão sem regressão: `ActiveConnCount` e a
  contagem por conexão continuam vindo de `activeConns`, que não
  muda; só o dado publicado no Valkey ganha granularidade.
- Ação em voo: coberto por serialização da mailbox (seção 5), testado
  com um teste de integração dedicado.
- Este ADR é a peça exigida antes de implementar.
- `tableconn` continua nunca decidindo auto-kick — só ganhou uma
  dimensão a mais de display state.
