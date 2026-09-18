# Ranking mensal (período no leaderboard) + seletor de métrica

Issues relacionadas: #294 (ligas sazonais — este plano entrega o valor central dela sem o
maquinário de promoção/rebaixamento), #291/#324 (temporadas de conquistas — compartilham a mesma
noção de período, mas não dependem deste trabalho).

## Problema

1. O ranking é vitalício e global por modo (`poker_leaderboard_stats`, um item por `(playerID, mode)`,
   contadores só crescem). Quem começa hoje nunca alcança quem joga desde o início — não há motivo de
   retorno recorrente.
2. O backend já suporta três métricas (`hands_won`, `hands_played`, `win_rate`) mas a UI nunca manda
   `metric` (`ui/src/lib/api/gamification.ts`), então o board é sempre `hands_won` e o jogador não tem
   como reordenar.

## Decisão de desenho

Um item por `(playerID, mode, período)`, **reusando os três GSIs existentes**: a chave de partição do
GSI passa a ser `<mode>` (vitalício) ou `<mode>#<YYYY-MM>` (mensal). Consequências:

- Zero mudança de CDK — nenhum GSI novo, nenhuma tabela nova.
- `Top`, `RankOf`, `countGSI` e o rank mirror funcionam sem alteração de lógica: só recebem outra
  string de board.
- O board de cada mês fica consultável para sempre (histórico), sem job de arquivamento.
- Custo: 2 escritas por assento/mão em vez de 1 (o vitalício + o do mês). Com item < 1KB e projeção
  `ALL` nos 3 GSIs, ~4 WCU → ~8 WCU por assento/mão. O teto é pinado por
  `TestRecordHandWriteBudget`, na convenção do #204.

Alternativas descartadas: item único com atributos mensais + GSIs novos (custo parecido, perde o
histórico ao virar o mês, exige mudança de CDK); snapshot mensal sem escrita por mão (não dá para
ordenar por GSI, logo `Top` viraria scan).

### Chave de período

`YYYY-MM` em BRT (`time.FixedZone("BRT", -3h)`), mesma convenção de `dailyreward/store.go`. Em UTC o
mês viraria às 21h do dia anterior no horário do jogador.

### Piso do win_rate

`MinHandsForWinRateRank = 100` vale igual no board mensal — um jogador ativo (~30 mãos/sessão) passa
disso com folga dentro de um mês. Sem constante nova.

### Default

- Servidor: `period=all` (compatibilidade — CLI e mobile continuam vendo o que veem hoje).
- Web: manda `period=month` explicitamente; "Geral" continua a um clique.

## Entregas

### api/

1. `internal/leaderboard/period.go` — `PeriodAll`/`PeriodMonth`, `MonthKey(now)`, `normalizePeriod`,
   `boardKey(mode, periodKey)`, `statsSK(mode, periodKey)`.
2. `internal/leaderboard/store.go` — todo método passa a receber `periodKey`; sk e chave de GSI
   derivadas dele. Nenhuma outra mudança de lógica.
3. `internal/leaderboard/service.go` — `Board{Mode, Metric, Period}` substitui os pares
   `(mode, metric)` posicionais; `RecordHand`/`RecordUnlocks` escrevem nos dois escopos
   (vitalício + mês corrente) via um `now func() time.Time` injetável.
4. `internal/leaderboard/rankmirror.go` — chave do mirror passa a incluir o board (mês novo = board
   novo = um rebuild, não um mirror errado).
5. `internal/api/v1/leaderboard.go` — query param `period` em `/leaderboard` e `/leaderboard/me`.
6. `internal/api/v1/player.go` — `leaderboardRanker` acompanha a assinatura; showcase usa `PeriodAll`.
7. Testes: budget de escrita por mão nos dois escopos, chave de mês em BRT (virada às 00:00 BRT),
   período inválido rejeitado, `Top`/`MyRank` consultando a partição certa.

### ui/

1. `lib/api/gamification.ts` — `period` e `metric` nos dois fetches e nas query keys.
2. `(app)/leaderboard/page.tsx` — `FilterGroup` de período (Mensal padrão · Geral) e de métrica
   (Vitórias · Mãos jogadas · Aproveitamento), reusando o componente que já existe.
3. Estado vazio do mês ("ninguém pontuou neste mês ainda") e dica do piso de 100 mãos quando a
   métrica é aproveitamento e o jogador está abaixo dele.
4. Testes: cada filtro refaz exatamente uma consulta, orçamento de 2 GETs por abertura preservado,
   axe sem violação nova.

### cdk/

Nada.
