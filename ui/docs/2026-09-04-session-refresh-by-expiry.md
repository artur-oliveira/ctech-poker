# Renovar a sessão por expiração, e suspender em background (#231)

## O problema

`useSessionKeepAlive` renovava o token a cada 4 minutos, sem olhar visibilidade nem expiração.
Com token de 15 minutos, uma aba escondida gastava até **15 refreshes/hora** sem ninguém olhando —
e ainda por cima refazia um refresh a cada `visibilitychange` e a cada `online`, mesmo com o token
recém-renovado.

## Como funciona agora

- **Agendamento por `exp`.** `tokenExpiryMs()` lê o `exp` do access token (JWT) e
  `nextRefreshDelayMs()` devolve `exp - TOKEN_REFRESH_MARGIN_MS - agora`. Um `setTimeout` armado
  nesse instante gasta **no máximo um refresh por token**. Ler um claim não é confiar nele: o `exp`
  decide apenas *quando* pedir um token novo — autorização continua sendo do servidor, e um `exp`
  mentiroso simplesmente cai no caminho de 401 que já existe.
- **Margem é o botão de calibração.** `TOKEN_REFRESH_MARGIN_MS` (60s) absorve endpoint de token
  lento e relógio de cliente adiantado/atrasado — o `Date.now()` do navegador não é o do IdP.
- **Fallback sem `exp`.** Token do mock, token opaco ou payload malformado voltam à cadência fixa
  de `TOKEN_REFRESH_INTERVAL_MS`, medida a partir da **última tentativa** de refresh
  (`lastRefreshAtMs`, escrito por todos os chamadores de `getOrRefreshSession` — keep-alive,
  interceptor de 401, recuperação do socket). Um refresh que outra pessoa acabou de fazer adia o
  keep-alive em vez de ser duplicado por ele.
- **Suspenso em background.** Com a aba escondida nenhum timer fica armado. Voltar renova **uma**
  vez, e só se o token estiver vencido/dentro da margem; caso contrário apenas rearma o timer. O
  mesmo vale para `online`. Pausar não pode custar caro na volta.
- **Rearme pelo resultado.** O timer é rearmado no `finally` da tentativa (não só na troca de
  token): um refresh que falhou numa oscilação de rede deixa o token intacto, e sem isso o
  keep-alive morreria ali. `subscribeAccessToken` também rearma, porque um token novo significa uma
  expiração nova para agendar.

## Coalescência

Dentro de uma aba, `getOrRefreshSession` já compartilha uma única promise entre todos os
chamadores — isso não mudou. **Entre abas não há coalescência**: cada aba guarda o próprio token em
memória (não em `localStorage`, por decisão de segurança), então uma aba não pode usar o token que
outra buscou. Um lock por `BroadcastChannel` só evitaria o refresh simultâneo, não a necessidade de
cada aba ter o seu token — ficou de fora deliberadamente.

## Medição

`sessionRefreshCount()` conta os refreshes da sessão do navegador, no mesmo espírito de
`purchasePollCount()`: o app não tem sink de métricas, e esse contador é o que torna "refreshes por
hora" assertável em teste e legível no console.

## Como validar

`npx vitest run src/lib/auth/session.test.tsx --maxWorkers=2`. Os casos novos cobrem: agendamento
pelo `exp` (nada dispara onde o intervalo antigo teria disparado três vezes), uma hora escondida
custando **zero** refreshes, volta/reconexão sem renovar com token fresco e renovando com token
vencido, e o fallback sem `exp`.
