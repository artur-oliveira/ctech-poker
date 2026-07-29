# Spec — Avatares de jogador + 4 features seguintes

Data: 2026-07-28 · Estado: proposta, não implementada · Escopo: `api/`, `ui/`, `cdk/`

Referência de estado atual: `docs/README.md`. Nada aqui está construído.

---

## Feature 1 — Foto de perfil (avatar) no assento e no perfil público

### Problema

Identidade de assento hoje é só nome + iniciais. `initials()` (`ui/src/lib/utils.ts:63`) é o único
helper compartilhado, com duas duplicatas ad-hoc (`ui/src/app/profile/page.tsx:47`,
`ui/src/app/page.tsx:179`). `hand.SeatView` (`api/internal/engine/hand/snapshot.go`) e
`pokerproto.Seat` carregam exatamente `player_id` + `name` — nenhum campo de imagem.

### Decisão de arquitetura (tomada)

Avatar é **poker-local**: bucket próprio, servido pela distribuição CloudFront que já existe.

**Dívida explícita:** `ctech-account` já tem `AvatarURL` (`api/internal/domain/user/model.go:13`,
exposto em `GET /account/profile`), hoje preenchido apenas pelo login Google, e o escopo `Profile`
literalmente diz "Ver nome e foto de perfil". A longo prazo esse é o dono correto — serve todos os
produtos CTech. Não foi escolhido agora porque exigiria duas repos, um endpoint de upload no account
e uma dependência nova poker → API do account (hoje o poker só consome JWKS; `jwtverify.Claims` não
tem claim `picture`).

**Caminho de migração:** a leitura no poker é sempre "o perfil tem um avatar ou não". Quando o
account virar dono, só o *writer* muda (o upload passa a ser lá, e o poker espelha a URL no connect
igual já faz com `name`). `SeatView`, showcase e histórico não mudam.

### Storage e entrega

```
Bucket:   <env>-ctech-poker-avatars     (privado, BLOCK_ALL, OAC, S3_MANAGED)
Behavior: /avatars/*  na distribuição atual  ->  esse bucket, CACHING_OPTIMIZED
Chave:    av/<userID>/<version>.jpg
URL:      https://poker[-env].aoctech.app/avatars/av/<userID>/<version>.jpg
```

Três razões para bucket separado servido pela **mesma** distribuição:

1. O bucket do frontend é implantado com `aws s3 sync out/ --delete`
   (`.github/workflows/frontend.yml`) — qualquer objeto de usuário lá é apagado a cada deploy.
2. Mesma origem = **CSP não muda**. A política atual já permite `img-src 'self' data:`
   (`cdk/lib/frontend-stack.ts`). Distribuição nova exigiria mexer no CSP.
3. Chave versionada + `Cache-Control: public, max-age=31536000, immutable` = zero invalidação de
   CloudFront no upload. Trocar a foto incrementa `version`, que é uma URL nova.

A CloudFront Function de rewrite só age em caminhos sem extensão, então `.jpg` passa intacto —
confirmar em teste.

### Modelo de dados

`poker_player_profiles` (`api/internal/player/model.go`) ganha:

```go
AvatarKey     string `dynamodbav:"avatar_key,omitempty"`     // "av/<id>/3.jpg"; vazio = sem foto
AvatarVersion int    `dynamodbav:"avatar_version,omitempty"` // monotônico, nunca reusado
AvatarBlocked bool   `dynamodbav:"avatar_blocked,omitempty"` // moderação; ver abaixo
```

Guardar **chave**, não URL: o host vem de config, então mudar de domínio ou CDN não exige migração
de dados. A URL é montada na borda de serialização.

### API

**`POST /v1.0/players/me/avatar`** — `multipart/form-data`, campo `image`.

Os bytes **passam pelo servidor** de propósito. Presign direto no S3 seria menos código, mas deixaria
entrar arbitrário: EXIF com geolocalização, PNG bomba de descompressão, SVG com script, GIF animado
piscando na mesa. Re-encodar no servidor resolve os quatro de uma vez, sem lib nenhuma —
`image/jpeg` + `image/png` da stdlib já bastam. Este é um limite de confiança; não simplificar.

Pipeline, nesta ordem:

1. Corpo limitado a **2 MB** (limite do Fiber, antes de qualquer leitura).
2. `image.DecodeConfig` primeiro: rejeita > 4096×4096 **antes** de alocar os pixels (bomba de
   descompressão). Rejeita formato que não seja `jpeg` ou `png` — o formato vem do decoder, não do
   `Content-Type` enviado pelo cliente. GIF fora: avatar animado na mesa é distração e vetor de abuso.
3. Decodifica, corta quadrado pelo centro, redimensiona para **192×192**, re-encoda JPEG q85.
   Um tamanho só: cobre o assento (28–52 px), o `ProfileMenu` (32 px) e o showcase (68 px) em 2x.
   Re-encodar descarta todo metadado por construção — nada de biblioteca de EXIF.
4. `PutObject` com `version+1`, `Cache-Control` imutável, `Content-Type: image/jpeg`.
5. Atualiza o perfil, depois **apaga a chave antiga** (best-effort; falha só deixa lixo barato).
6. Rate limit **5 uploads/hora/jogador** (`internal/api/v1/ratelimit.go` já tem o padrão).

Resposta: o `playerResponse` de sempre, agora com `avatar_url`.

**`DELETE /v1.0/players/me/avatar`** — limpa `avatar_key`, apaga o objeto, volta para iniciais.

**Onde `avatar_url` passa a aparecer:**

| Superfície                                             | Campo        | Observação                                                                                              |
|--------------------------------------------------------|--------------|---------------------------------------------------------------------------------------------------------|
| `GET/POST /v1.0/players/me`                            | `avatar_url` | próprio                                                                                                 |
| `GET /v1.0/players/:id/showcase`                       | `avatar_url` | público, já é opt-in por `showcase_public`                                                              |
| Snapshot da mesa (`SeatView` → `pokerproto.Seat`)      | `avatar_url` | por assento                                                                                             |
| `GET /v1.0/players/me/hands[/:id]` (`OpponentSummary`) | `avatar_url` | histórico                                                                                               |
| **`GET /v1.0/hand-shares/:token`**                     | **nunca**    | mão compartilhada é anonimizada com alias; avatar deanonimiza. Precisa de teste que garanta a ausência. |

`AvatarBlocked` faz o servidor omitir `avatar_url` em todas as superfícies — cai para iniciais.

### Caminho no motor

O ator recebe o nome no connect do WS via `table.SetNameCmd` (`internal/api/v1/tablews.go`). Estender
esse mesmo comando para carregar a identidade inteira (`SetIdentityCmd{Name, AvatarURL}`) em vez de
criar um segundo comando; `hand.Player` ganha `AvatarURL` ao lado de `Name`.

⚠️ Fiber faz hijack da conexão: copiar a string vinda do contexto **antes** da goroutine do socket —
mesma pegadinha que já causou o bug de "no state seeded".

### Frontend

- `ui/src/components/ui/avatar.tsx` ganha `AvatarImage` (hoje só existe `Avatar` +
  `AvatarFallback`). `next/image` não serve: `images.unoptimized` + export estático, então `<img>` puro.
- Um componente `PlayerAvatar({name, avatarUrl, size})` que renderiza a imagem com fallback para
  `initials(name)` — tanto quando não há URL quanto no `onError`. **Substituir as duas duplicatas
  ad-hoc** de iniciais em `app/profile/page.tsx:47` e `app/page.tsx:179` por ele.
- Consumidores: `Seat.tsx:145`, `ProfileMenu.tsx:60`, `app/profile/page.tsx`, `hands/history`,
  `LastWinners`, `PlayerNoteDialog`.
- Upload no `ProfileMenu` (é onde o perfil já se edita — `/profile` é showcase público, não editor):
  `<input type="file" accept="image/jpeg,image/png">`, recorte e redução para 192×192 no **canvas do
  navegador** antes de enviar. Dois ganhos: o payload cai para ~20 KB, e o navegador já aplica a
  orientação EXIF ao rasterizar, então foto de celular não chega de lado. O servidor re-encoda de
  novo mesmo assim — o cliente nunca é o limite de confiança.
- **Decisão de design:** `.seat-avatar` hoje é `display: none` no desktop
  (`globals.css:2812`, "assentos são cartões de nome"); só aparece no anel vertical de retrato.
  Ligar um avatar de 28 px no assento desktop — sem foto o assento continua idêntico ao de hoje,
  porque o fallback é a inicial no mesmo espaço. Registrar em `ui/DESIGN.md § 5`.

### CDK

Bucket novo + behavior `/avatars/*` na distribuição existente; `PutObject`/`DeleteObject` para o role
da instância **só nesse bucket** (o role não tem, e não deve ter, escrita no bucket do frontend).
Regra de lifecycle não é necessária — o upload já apaga a chave anterior.

### Moderação

Foto enviada por usuário aparecendo na frente de outros jogadores é superfície de abuso, não detalhe.
Mínimo viável, sem construir painel de admin:

- `POST /v1.0/players/:id/avatar/report` — grava a denúncia, rate-limited por denunciante.
- `AvatarBlocked` no perfil: enquanto ligado, o servidor serve iniciais em toda superfície. Virar a
  flag é uma escrita no DynamoDB. **Honestidade:** não há fila de revisão nem ferramenta de operador
  nesta fatia; é uma alavanca manual. Se o volume crescer, o próximo passo é Rekognition
  `DetectModerationLabels` no upload, que encaixa exatamente no passo 3 do pipeline.

### Verificação

- Go: rejeição de dimensão via `DecodeConfig` sem decodificar; rejeição de formato não-jpeg/png;
  strip de EXIF (encodar com EXIF, ler o objeto de volta, garantir ausência); incremento de versão +
  delete da chave antiga; `avatar_url` presente no snapshot por assento; **ausente** em
  `hand-shares`; `AvatarBlocked` omite em todas as superfícies.
- UI: fallback para iniciais sem URL e em `onError`; recorte no canvas produz 192×192; o assento sem
  foto renderiza igual ao de hoje.
- Manual: enviar foto de celular em retrato e confirmar orientação; confirmar que a Function de
  rewrite do CloudFront não intercepta `/avatars/*.jpg`.

---

## Feature 2 — Run It Twice

Em all-in com ação encerrada, os jogadores envolvidos podem pedir que o restante do board seja
distribuído **duas vezes**, cada metade valendo metade do pote. Reduz variância sem mudar EV — é o
pedido mais recorrente de jogador regular e não existe hoje (`PLAN.md` lista como deferido).

**Por que encaixa bem aqui:** as duas runouts saem do **mesmo baralho já comprometido**. As cartas
`nextCard…nextCard+n` são a primeira runout e as seguintes são a segunda; o `RootCommitHash`
publicado antes da mão já prova as duas, sem primitiva criptográfica nova. É, provavelmente, o
melhor argumento de marketing do provably-fair que o produto tem.

- `engine/hand`: consentimento por jogador (todos os all-in envolvidos precisam aceitar), segunda
  runout, e `sidepots` aplicado duas vezes sobre metades do pote. Divisão ímpar de fichas vai para o
  primeiro jogador à esquerda do dealer — a mesma regra que o resto do motor já usa.
- `HandOutcome` passa a comportar dois boards; `ui/src/components/table/Board.tsx` renderiza dois
  runouts empilhados, e o replayer/histórico precisa acompanhar.
- Opção por sala (`run_it_twice_enabled`, default `false`), fixada na criação como as demais.
- Cuidado: interage com rabbit hunt (não oferecer os dois) e com o pacing de reveal já ajustado.

Escopo: motor + snapshot + UI de board/histórico. Nenhuma infra nova.

---

## Feature 3 — Badges de estilo públicos (opt-in)

`internal/pokerstats` já materializa VPIP/PFR/3-bet por jogador, hoje visível só para o próprio dono
(`SelfHudDialog`). Derivar disso um **badge de estilo** público e opcional — "Agressivo", "Rochedo",
"Maníaco", "Sólido" — a partir de faixas de VPIP/PFR, exibido no assento junto ao avatar e no showcase.

- Faixas em tabela de dados, não `if` espalhado, para poderem ser recalibradas sem deploy de lógica.
- **Piso de amostra** (ex.: 200 mãos) antes de mostrar badge; abaixo disso, nada. Badge derivado de 12
  mãos é ruído apresentado como informação.
- Opt-in por perfil (`playstyle_public`), no mesmo diálogo do showcase. Nunca expõe os números
  brutos, só o rótulo — número exato é vantagem competitiva e assimétrica.
- Não vaza nada novo: um oponente atento chega ao mesmo rótulo observando. A diferença é que fica
  simétrico entre quem tem HUD externo e quem não tem.

Escopo: agregação em `pokerstats`, campo no perfil, campo no `SeatView`, badge na UI. Emparelha
naturalmente com a Feature 1 — as duas juntas é que fazem o assento parecer uma pessoa.

---

## Feature 4 — Grid multi-mesa

Jogar 2–4 mesas simultâneas numa grade, com foco automático na mesa onde é a vez do jogador.

- O backend praticamente não muda: `useTableRealtime` já é por mesa e o gateway de lobby
  (`GET /v1.0/ws`, canal `user#<id>`) já é o lugar certo para o sinal de "é sua vez na mesa X".
- O trabalho real é frontend: layout de grade responsivo, uma instância de socket por mesa,
  **foco de teclado** e roteamento de atalhos para a mesa ativa, e som/notificação por mesa sem virar
  cacofonia.
- Precisa de time bank por mesa visível ao mesmo tempo — o `PerimeterTimer` já existe, o problema é
  hierarquia visual com 4 deles na tela.
- Limite duro de 4. Acima disso a experiência degrada e a pressão de decisão passa a favorecer bot.
- Interage com `RealityCheck`: sessão de 4 mesas conta tempo diferente de sessão de uma.

Escopo: quase tudo em `ui/`. Verificar consumo de memória e número de sockets antes de subir o limite.

---

## Feature 5 — Clubes / ligas privadas

Grupo persistente de jogadores com ranking próprio e temporadas, em cima do que já existe:
`roomstore` (salas privadas com `share_code`) + `leaderboard` (agregação e GSIs).

- `poker_clubs` (metadados, dono, código de convite) e `poker_club_members`. O padrão de código de
  convite com compare de tempo constante já está em `roomstore`/`tablews` — reusar, não reinventar.
- Ranking **escopado ao clube** por temporada. O `leaderboard` atual agrega global por GSI; escopo por
  clube quer um novo par PK/SK (`club#<id>` / métrica), não um GSI a mais na tabela global.
- Sala de clube = sala privada que já herda os membros, em vez de recircular link.
- Sandbox primeiro. Clube com dinheiro real levanta pergunta regulatória própria (organização de
  jogo entre pessoas determinadas), que é exatamente a área cinzenta de `OVERVIEW.md § 11` — não
  liberar em modo real sem o mesmo parecer.

Escopo: tabelas novas, um pacote `internal/clubs`, telas de clube. É o maior dos cinco.

---

## Ordem sugerida

1. **Avatares** — desbloqueia identidade de assento, e a Feature 3 depende dela para fazer sentido visual.
2. **Run It Twice** — contido no motor, e é o que mais aproveita o provably-fair que acabou de ficar pronto.
3. **Badges de estilo** — barato, reusa `pokerstats` inteiro.
4. **Grid multi-mesa** — só frontend, mas trabalhoso de acertar.
5. **Clubes** — maior, e o único que levanta pergunta regulatória nova.

### Planos de implementação (2026-07-29)

Cada feature tem plano próprio, escrito contra o código verificado. Os planos corrigem várias
afirmações desta spec — onde divergirem, **o plano vence**:

| # | Plano | Correção relevante à spec |
|---|-------|---------------------------|
| 1 | `docs/plans/2026-07-29-player-avatars.md` | upload é **presigned POST direto ao S3** + confirm, não POST no servidor; o aviso de hijack do Fiber não se aplica; há **uma** duplicata de iniciais, não duas; `.seat-avatar` é controlado por classe, não media query; a CloudFront Function nem é associada a behavior adicional |
| 2 | `docs/plans/2026-07-29-run-it-twice.md` | **não** há conflito com rabbit hunt (condições mutuamente exclusivas); consentimento é pré-declarado, não prompt por mão |
| 3 | `docs/plans/2026-07-29-playstyle-badges.md` | a derivação de badge **já existe** no cliente (`SelfHudDialog.tsx:25-37`) — é promoção para Go, não feature nova |
| 4 | `docs/plans/2026-07-29-multi-table-grid.md` | o sinal de "é sua vez" **não** vem do gateway de lobby; cada socket de mesa já o tem. Backend não muda em nada |
| 5 | `docs/plans/2026-07-29-clubs-and-private-leagues.md` | **uma** tabela com `pk`/`sk`, não duas |

## Não coberto aqui

Torneios/Sit & Go, spectator mode, mixed games (PLO/short deck), mental poker, staking, AR/VR. Ver
`future.md` e `future_analysis.md` — parte do backlog Fase 1–2 de lá já foi entregue em 2026-07-26→28.
