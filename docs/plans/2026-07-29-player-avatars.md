# Plano de Implementação — Avatar de jogador (assento + perfil público)

Data: 2026-07-29 · Escopo: `api/`, `ui/`, `cdk/` · Spec: `docs/specs/2026-07-28-player-avatars-and-next-features.md` (Feature 1)

## 📌 Contexto

Identidade de assento hoje é nome + iniciais. `Seat.tsx:145-146` renderiza
`<Avatar className="seat-avatar"><AvatarFallback>{isViewer ? 'EU' : initials(seat.name)}</AvatarFallback></Avatar>`,
e `.seat-avatar` é `display: none` (`globals.css:2812-2814`). `hand.Player` (`hand.go:49-84`),
`SeatView` (`snapshot.go:107-129`) e `pokerproto.Seat` (`poker.proto:13-43`) carregam `Name` e nenhum
campo de imagem. `PlayerProfile` (`player/model.go:19-30`) também não.

Este plano constrói o caminho inteiro: upload → S3 → CloudFront → perfil → snapshot → assento.

## 📌 Nota de Arquitetura: o poker é dono temporário

`ctech-account` já tem `AvatarURL` e um escopo `Profile` que literalmente diz "Ver nome e foto de
perfil" — é o dono correto a longo prazo, porque serve todos os produtos CTech. Não é ele agora porque
exigiria endpoint de upload lá, duas repos no mesmo commit, e uma dependência nova poker → API do
account (hoje o poker só consome JWKS; `jwtverify.Claims` não tem claim `picture`).

**Caminho de migração preservado:** a leitura no poker é sempre "o perfil tem avatar ou não", e o
avatar chega no assento pelo mesmo comando que já carrega o nome. Quando o account virar dono, só o
*writer* muda. `SeatView`, showcase e histórico ficam iguais.

### Correções à spec, verificadas no código

Quatro afirmações da spec estavam erradas ou desatualizadas:

1. **O aviso de hijack do Fiber não se aplica aqui.** A spec manda copiar a string antes da goroutine
   do socket. O `SetNameCmd` de `tablews.go:367-372` lê o nome de `players.GetOrCreate` — DynamoDB,
   *dentro* da goroutine — não do `fiber.Ctx`. Os únicos `strings.Clone` necessários já existem em
   `tablews.go:248-249` (`tableID`, `remoteIP`). O avatar segue o mesmo caminho do nome, então não
   adiciona risco novo. (`app.go:91` também já usa `Immutable: true`.)
2. **Há uma duplicata de iniciais, não duas.** `ui/src/app/profile/page.tsx:46-48` usa
   `(name || '?').slice(0, 2).toUpperCase()` — **comportamento diferente** de `initials()`
   (`lib/utils.ts:63-67`, que faz primeira + última inicial). "Ana Beatriz" hoje é `AN` no showcase e
   `AB` no assento. Isso é um bug de consistência a corrigir junto. `ui/src/app/page.tsx:177-179` é
   mock estático de marketing (array literal de nomes), **não** consome API — deixar quieto.
3. **`.seat-avatar` não é controlado por media query.** `globals.css:2812-2814` é incondicional, e
   `globals.css:5517-5522` religa via a *classe* `.stage-v-ring`, aplicada em JS por
   `TableStage.tsx:114` conforme `matchMedia('(orientation: portrait) and (max-width: 1023px)')`
   (`TableStage.tsx:19`). Ligar no desktop é editar a regra de `:2812`, não uma media query.
4. **A CloudFront Function não intercepta `/avatars/*` de jeito nenhum.** Ela está associada apenas ao
   *default behavior* (`frontend-stack.ts:113`); um behavior novo não a herda. Além disso o próprio
   código retorna cedo para qualquer URI com extensão (`frontend-stack.ts:54`,
   `/\.[^/]+$/.test(uri)`). Preocupação dupla, risco zero — mas o teste de CDK deve travar isso.

### O que o `api/` não tem hoje

- **Nenhum multipart, nenhum decode de imagem.** Zero ocorrências de `c.FormFile` ou `image/` em
  `api/`.
- **Nenhum cliente S3 no grafo Fx** (`app.go:50-75`). O SDK já é dependência direta
  (`go.mod:12`, `service/s3 v1.106.0`) e existe um único uso, em `cmd/archiver/main.go:24-35`
  (interface `s3Putter` + `realS3Putter`), que é Lambda, fora do Fx. **Reusar aquele formato de
  interface** no pacote novo em vez de injetar `*s3.Client` cru — é o que torna o handler testável sem
  rede.
- **`BodyLimit` não é configurado** em `app.go:82-110`, ou seja, vale o default do Fiber para *todas* as
  rotas. Com upload direto ao S3 isso deixa de importar aqui: os dois corpos que chegam ao Fiber são
  JSON de uma linha. O limite de 2 MB é imposto pelo S3, via `content-length-range` na policy.
- **CORS permite `GET, POST, DELETE, OPTIONS`** (`app.go:116`) — sem PUT/PATCH. `POST` + `DELETE`, como
  planejado, não exigem mudança de CORS.
- **O rate limiter só tem chave por IP** (`ratelimit.go:97-101`, `ipKey`). Limitar upload por IP é
  errado: NAT compartilhado (faculdade, operadora móvel) puniria terceiros. Precisa de `playerKey`.

### Decisão: upload direto ao S3, não pelo servidor

Os bytes **não** passam pelo servidor. O cliente pede uma URL assinada, envia direto ao S3, e depois
confirma. A instância EC2 é 1–3 nós atrás de ALB (`cdk/CLAUDE.md`) servindo WebSocket de mesa em tempo
real — subir imagem por ali gasta banda e memória de um processo cujo trabalho é latência de jogo.

**Isto remove o re-encode server-side, que era a defesa única do plano anterior.** Uma URL assinada
crua aceitaria SVG com script, EXIF com geolocalização, PNG bomba e GIF animado. Não é aceitável, e
não é o que este plano faz: as quatro defesas passam a ser distribuídas, e **cada uma delas é
obrigatória** — tirar qualquer uma reabre um dos quatro vetores.

| Vetor | Defesa neste desenho |
|-------|----------------------|
| Tamanho arbitrário | `content-length-range` na policy do presigned POST — **o S3 recusa**, o servidor nunca vê |
| Formato mentiroso (SVG, WebP, HTML) | `image.DecodeConfig` no confirm, sobre GET com `Range` dos primeiros 64 KB — formato vem do decoder |
| Bomba de descompressão | mesmo `DecodeConfig`: dimensão declarada > 4096² recusa **sem alocar pixels** |
| EXIF com geolocalização | confirm recusa JPEG que tenha marcador `APP1`/`Exif` — canvas do navegador nunca emite um |
| SVG servido como HTML na origem do app | `Content-Type` reescrito pelo servidor no `CopyObject` + `nosniff` no CloudFront |

A validação custa um GET com `Range` de 64 KB e um `DecodeConfig` de cabeçalho — não decodifica pixel,
não redimensiona, não re-encoda. É ~1 ms e alguns KB de banda, contra 2 MB de upload e um resample no
processo do jogo. É a troca que justifica o desenho.

**EXIF: recusar, não remover.** Sem os pixels na mão o servidor não pode limpar metadado; podia
reescrever os segmentos JPEG, mas são ~40 linhas de parser de marcador para resolver um caso que o
cliente honesto nunca produz — `canvas.toBlob()` rasteriza e descarta todo metadado por construção.
Então o confirm **recusa** e a mensagem diz para reenviar. Quem burla só expõe a própria localização,
e ainda cai no `Content-Type` reescrito.

---

## Fase 1 — Infra

### T1 — Bucket + behavior (`cdk/lib/frontend-stack.ts`)

```
Bucket:   <env>-ctech-poker-avatars   (BLOCK_ALL, S3_MANAGED, RETAIN em prod)
Prefixos: up/<userID>/<version>.jpg   quarentena, o cliente escreve aqui
          av/<userID>/<version>.jpg   publicado, só o servidor escreve (CopyObject)
Behavior: /avatars/*  ->  esse bucket com originPath: '/av'
URL:      https://poker[-env].aoctech.app/avatars/<userID>/<version>.jpg
```

Espelhar o bucket do frontend de `frontend-stack.ts:31-38`. Reusar o **mesmo OAC** de `:39-41` e a
**mesma distribuição** de `:103-121`.

**`originPath: '/av'` é controle de segurança, não estética de URL.** Com ele o CloudFront não tem
como alcançar `up/` — o objeto em quarentena é inatingível pela CDN até o servidor copiá-lo. Sem
`originPath`, `/avatars/up/<id>/7.jpg` serviria bytes não validados na origem do app.

**Lifecycle rule: expirar `up/` em 1 dia.** É o que limpa upload abandonado (presign emitido, POST
feito, confirm nunca chega) sem nenhuma lógica de órfão no servidor. Não expirar `av/` nunca — lá está
a foto viva.

**CORS no bucket** (`s3.Bucket.cors`), senão o navegador bloqueia o POST direto:

```
allowedMethods: [POST], allowedOrigins: [https://<domainName>], allowedHeaders: ['*'], maxAge: 3000
```

Só `POST`, só a origem do site. Não usar `*` em `allowedOrigins`: qualquer página da internet poderia
gastar o presign de um jogador logado se conseguisse o token.

**`responseHeadersPolicy` no behavior de `/avatars/*`:** política nova e mínima, com
`contentTypeOptions: {override: true}` (`X-Content-Type-Options: nosniff`). Não reusar o
`securityHeaders` de `:60-89` — CSP e HSTS em resposta de imagem não fazem nada, e
`X-Frame-Options: DENY` numa imagem é ruído. Mas `nosniff` **é** necessário: as imagens são servidas na
**mesma origem** do app, então um objeto que escapasse com corpo HTML e o navegador farejando tipo
seria XSS armazenado. O `Content-Type` já é reescrito pelo servidor no `CopyObject` (T5); `nosniff` é a
segunda tranca.

`additionalBehaviors` em `:115` hoje é `Object.fromEntries(API_PATH_PATTERNS.map(...))` — um único
behavior (`/v1.0/*`, `constants.ts:31`). Passa a fazer spread de `/avatars/*` junto. O
`responseHeadersPolicy` de `:112` não se aplica a behavior adicional por default; **não** aplicar o
`securityHeaders` aqui — CSP e HSTS em resposta de imagem não fazem nada, e `X-Frame-Options: DENY`
numa imagem é ruído.

Três razões para bucket separado servido pela **mesma** distribuição:

1. O bucket do frontend é implantado com `aws s3 sync out/ --delete`
   (`.github/workflows/frontend.yml`) — objeto de usuário lá morre no próximo deploy.
2. Mesma origem para **leitura** = `img-src` não muda. `frontend-stack.ts:83` já permite
   `img-src 'self' data:` e a foto sai de `https://<domainName>/avatars/...`. Distribuição nova exigiria
   mexer no `img-src` também.
3. Chave versionada + `Cache-Control: public, max-age=31536000, immutable` = **zero invalidação**.
   Trocar a foto incrementa `version`, que é outra URL.

### T1b — CSP para o POST direto (`cdk/lib/frontend-stack.ts`, `bin/poker.ts`)

⚠️ **Sem isto o upload quebra em produção e funciona em dev** — o pior modo de falha possível. O POST
vai para `https://<bucket>.s3.<region>.amazonaws.com`, que **não** é `'self'`, e o CSP de
`frontend-stack.ts:84` só libera `'self'`, o domínio de auth e `extraConnectSrc`.

O gancho já existe: `extraConnectSrc: string[]` (`frontend-stack.ts:18`), hoje com um item
(`CLOUDFLARE_CHALLENGE_SRC`, `bin/poker.ts:114`). Acrescentar o endpoint regional do bucket lá, como
constante em `lib/constants.ts` (a convenção do repo proíbe nome de recurso AWS inline).

Duas armadilhas:

- **`connect-src`, não `form-action`.** Vale enquanto o envio for `fetch`/`XHR` com `FormData`. Se
  alguém trocar por `<form method="post">` de verdade, a diretiva que passa a valer é `form-action`,
  que hoje nem está no CSP — e aí `default-src 'self'` não cobre, porque `form-action` não herda de
  `default-src`. Manter `fetch`.
- O endpoint é o **virtual-hosted regional** (`<bucket>.s3.us-east-1.amazonaws.com`), que é o que o
  SDK v2 assina. Colocar `s3.amazonaws.com` genérico não casa com o host e o navegador bloqueia.

### T2 — IAM (`cdk/lib/api-stack.ts`)

Statements novos, no formato dos dois grants S3 que já existem em `:145-152`:

```
s3:PutObject                            em  ${avatarsBucketArn}/up/*   (assinar o presigned POST)
s3:GetObject, s3:DeleteObject           em  ${avatarsBucketArn}/up/*   (validar e limpar quarentena)
s3:PutObject, s3:DeleteObject           em  ${avatarsBucketArn}/av/*   (publicar e apagar versão antiga)
```

**Só esse bucket, só esses prefixos.** O role da instância não tem — e não deve ter — escrita no bucket
do frontend. **Sem `s3:GetObject` em `av/*`**: quem lê o publicado é o CloudFront via OAC. A instância
lê apenas quarentena, e só para validar.

O `GetObject` em `up/*` é a diferença em relação ao desenho anterior, e é o que o `CopyObject` de T5
exige (copiar precisa ler a origem).

O bucket entra em `PokerStackProps`/`bin/poker.ts` no mesmo formato de `deploymentsBucketName` e
`logsBucketName`.

Testes de CDK: behavior `/avatars/*` existe, **sem** `FunctionAssociations`, **com** `originPath` `/av`;
policy de S3 não referencia o bucket do frontend e não tem `GetObject` em `av/*`; CORS do bucket não tem
`*` em `AllowedOrigins`; lifecycle rule cobre `up/` e não `av/`.

## Fase 2 — Backend

### T3 — Campos no perfil (`internal/player/`)

`model.go:19-30` ganha:

```go
AvatarKey     string `dynamodbav:"avatar_key,omitempty" json:"-"`     // "av/<id>/3.jpg"; vazio = sem foto
AvatarVersion int    `dynamodbav:"avatar_version,omitempty" json:"-"` // monotônico, nunca reusado
AvatarBlocked bool   `dynamodbav:"avatar_blocked,omitempty" json:"-"` // moderação; ver T8
```

`json:"-"` nos três: o que sai na API é `avatar_url` montado, nunca a chave. Guardar **chave** e não URL
é o que evita migração de dados quando o domínio ou o CDN mudar.

`store.go` não tem update genérico — cada campo tem seu `Set*` com mapa literal de atributos
(`SetName:64`, `SetWalletMode:81`, `SetDeckVariant:98`, `SetShowcase:115`). Seguir o padrão:

```go
// SetAvatar grava chave e versão juntas: versão sem chave, ou o contrário, é
// estado impossível de servir.
func (s *Store) SetAvatar(ctx context.Context, userID, key string, version int) error
func (s *Store) ClearAvatar(ctx context.Context, userID string) error
```

Estender a interface `profileStore` em `service.go:28-36` — os handlers dependem dela, não do store
concreto.

### T4 — Montagem da URL

Um lugar só, e nunca em dois:

```go
// AvatarURL monta a URL pública a partir da chave. Vazio quando não há foto ou
// quando a moderação bloqueou — o cliente cai para iniciais nos dois casos, que
// é o mesmo estado visual e não vaza qual dos dois aconteceu.
func AvatarURL(p *PlayerProfile, baseURL string) string
```

`baseURL` vem de `config` (`AVATAR_BASE_URL`, tipo `https://poker.aoctech.app/avatars`), via SSM como
os outros parâmetros de `api-stack.ts:136-144`. Vazio em dev = avatar desligado, sem quebrar nada.

**`AvatarBlocked` retorna string vazia.** A checagem vive aqui, não em cada superfície — omitir num
lugar e esquecer em outro é como um flag de moderação vaza.

### T5 — Duas rotas: assinar e confirmar

Ambas no grupo autenticado de `player.go:52`. Nenhum byte de imagem atravessa o Fiber, então **nem
multipart nem `BodyLimit` são necessários** — os dois corpos são JSON pequeno.

#### `POST /v1.0/players/me/avatar/upload-url`

1. **Rate limit 5/hora por jogador** — aqui, não no confirm. É onde o custo é criado: um presign
   emitido é uma autorização de escrita no S3. Exige `playerKey` novo em `ratelimit.go`, no formato de
   `ipKey:97-101`, lendo `c.Locals(localsUserID)` (`auth.go:11,29`); limitador construído em
   `router.go:68-70` junto dos outros três.
2. `version := profile.AvatarVersion + 1`, chave `up/<userID>/<version>.jpg`. **O servidor escolhe a
   chave** — o cliente não a envia e não pode influenciá-la, senão escreveria sobre a foto de outro
   jogador.
3. `s3.NewPresignClient(...).PresignPostObject` (existe no `service/s3 v1.106.0`, `go.mod:12`), com
   `Conditions`:

```
content-length-range  1 .. 2097152          // o S3 recusa antes de gravar
Content-Type          eq "image/jpeg"       // metadado do objeto em quarentena; não é validação
key                   eq up/<userID>/<v>.jpg
Expires               120s
```

   `Content-Type` na condição **não valida nada** — o S3 não fareja o corpo, guarda o que foi
   declarado. Está ali só para o objeto em quarentena não nascer `application/octet-stream`. A validação
   real é o passo 2 do confirm.

Resposta: `{url, fields, version}` — `fields` são os campos da policy, que o cliente joga no `FormData`
antes do arquivo.

#### `POST /v1.0/players/me/avatar/confirm` — `{version}`

Ordem exata; cada passo depende do anterior ter recusado o que devia:

1. **`version` tem que ser `AvatarVersion + 1`.** Recusar qualquer outro valor. Sem esta checagem o
   confirm publica uma chave `up/` arbitrária de versão antiga que o jogador ainda tenha lá.
2. **`GetObject` com `Range: bytes=0-65535`** e `image.DecodeConfig` sobre o resultado. Recusa:
   - formato que não seja `jpeg` ou `png` — **o formato vem do decoder, nunca do `Content-Type` do
     objeto**, que o cliente escolheu. GIF fora por decisão: avatar animado na mesa é distração e vetor
     de abuso;
   - dimensão declarada > 4096×4096, **sem alocar pixels** — a defesa contra bomba de descompressão só
     funciona porque `DecodeConfig` lê cabeçalho e para;
   - JPEG com segmento `APP1`/`Exif` (ver decisão acima: recusar, não remover);
   - `ContentLength` do `GetObject` > 2 MB (cinto e suspensório sobre o `content-length-range`).

   Objeto ausente = 404, não 500: é o caso normal de "o POST ao S3 falhou e o cliente confirmou mesmo
   assim".
3. **`CopyObject`** `up/<id>/<v>.jpg` → `av/<id>/<v>.jpg` com `MetadataDirective: REPLACE`,
   `ContentType: image/jpeg`, `CacheControl: public, max-age=31536000, immutable`.

   `REPLACE` é o ponto de virada de confiança: o metadado do objeto publicado é **escrito pelo
   servidor**, então o `Content-Type` que o cliente declarou no POST não sobrevive à publicação. É o que
   fecha o vetor de SVG/HTML servido na origem do app.
4. **`SetAvatar`** (chave `av/...` + versão).
5. Best-effort, nesta ordem, e **nenhum deles fatal**: `DeleteObject` de `up/<id>/<v>.jpg`,
   `DeleteObject` de `av/<id>/<v-1>.jpg`.

   Ordem importa: se o delete do antigo vier antes do `SetAvatar` e o `SetAvatar` falhar, o perfil
   aponta para objeto que não existe. E o `up/` pode até vazar — a lifecycle rule de T1 o pega em 1 dia.

Resposta: o `playerResponse` de sempre.

#### `DELETE /v1.0/players/me/avatar`

`ClearAvatar`, apaga o objeto `av/`, volta para iniciais. CORS já permite DELETE (`app.go:116`).

#### Cliente S3 no Fx

Novo pacote com interface pequena no formato de `cmd/archiver/main.go:24-35` (`s3Putter` +
`realS3Putter`) — **presign, get-range, copy, delete**, quatro métodos. Injetada via interface, não
`*s3.Client` cru: é o que deixa o confirm testável sem rede, e o teste de EXIF de Fase 5 depende disso.

### T6 — Onde `avatar_url` aparece

| Superfície                                             | Onde editar                     | Observação                             |
|--------------------------------------------------------|---------------------------------|----------------------------------------|
| `GET/POST /v1.0/players/me`                            | `player.go:265-276`             | `playerResponse` é `fiber.Map`         |
| `GET /v1.0/players/:id/showcase`                       | `player.go:243-248`             | mapa inline **separado**, não reusa o de cima |
| Snapshot da mesa                                       | `snapshot.go:107-129`, `:219-229` | por assento                          |
| `GET /v1.0/players/me/hands[/:id]`                     | `sessionlog/store.go:83-92`     | `OpponentSummary`; populado em `app.go:327-345` |
| **`GET /v1.0/hand-shares/:token`**                     | **nunca**                       | ver abaixo                             |

⚠️ **Hand-shares anonimiza na *criação*, não na leitura.** `handshares.go:82-91` devolve o objeto
gravado verbatim; a anonimização acontece em `create` (`:36-80`) via `anonymizedOpponents:124-135` e
`anonymizedActions:137-175`, que emitem assento com `PlayerID: alias, Name: "Você"|"Jogador"`
(`:161-168`). Então o teste tem que travar o **caminho de criação**: se alguém acrescentar avatar ao
seat literal de `:165-168`, todas as mãos compartilhadas a partir dali deanonimizam retroativamente na
leitura. Alias + foto do jogador é o mesmo que não ter alias.

Notar que `player.go:243-248` e `:265-276` são **dois** construtores de resposta independentes — o
showcase não reusa `playerResponse`. Editar os dois; é exatamente o tipo de par que divergem.

### T7 — Caminho no motor

`SetNameCmd` já é o mecanismo para dado de perfil (não de motor) chegar ao `SeatView`, e é
server-authoritative por design (comentário em `commands.go:164-168`). **Estender, não duplicar:**

1. `commands.go:169-173` — `SetNameCmd` → `SetIdentityCmd{PlayerID, Name, AvatarURL, Reply}`.
2. `hand.go:288-299` — `SetPlayerNameForActor` → `SetPlayerIdentityForActor`, preservando o retorno
   `bool` de "mudou algo" que evita commit no-op (`actor.go:440` depende dele).
3. `hand.go:49-84` — `Player` ganha `AvatarURL string \`dynamodbav:"avatar_url,omitempty"\`` ao lado de
   `Name:51`. Persiste no estado, então sobrevive a troca de instância/lease.
4. `snapshot.go:107-129` — `SeatView.AvatarURL`, preenchido em `:219-229`.
5. `poker.proto:13-43` — `Seat` ganha `optional string avatar_url = 16;` (**16 é o próximo livre**,
   confirmado, sem `reserved`). Regerar Go + ts-proto. Conversão `tablews.go:819-835`.
6. Dois sites de dispatch, ambos já fazem `GetOrCreate` e já são os momentos certos:
   `tablews.go:367-372` (connect) e `buyin/service.go:224` (sentar).
7. `actor.go:432-458` — `handleSetName` → `handleSetIdentity`; o no-op de nome vazio em `:433` passa a
   ser "nome vazio **e** avatar vazio", senão jogador sem nome nunca recebe avatar.

## Fase 3 — Frontend

### T8 — Componentes

- `ui/src/components/ui/avatar.tsx` (15 linhas hoje, só `Avatar` + `AvatarFallback`) ganha
  `AvatarImage` embrulhando `Primitive.Image` do Base UI — o primitivo existe e não está embrulhado.
  `next/image` não serve: `images.unoptimized` + export estático.
- `PlayerAvatar({name, avatarUrl, size, isViewer})` — um componente, fallback para `initials(name)`
  quando não há URL **e** no `onError`. O Base UI `Avatar.Fallback` já cobre o caso de falha de carga.
- **Substituir a duplicata de `profile/page.tsx:46-48`** (que hoje faz `slice(0,2)` e divergia de
  `initials()`) por ele. `app/page.tsx:177-179` fica: é mock de marketing.
- Consumidores: `Seat.tsx:145-146`, `ProfileMenu.tsx:60`, `app/profile/page.tsx:46-48`,
  `hands/history`, `LastWinners`, `PlayerNoteDialog`.

### T9 — Upload no `ProfileMenu`

`ProfileMenu.tsx` é onde o perfil se edita (`/profile` é showcase público, não editor).
`<input type="file" accept="image/jpeg,image/png">`. O redimensionamento agora é **só** no cliente — o
servidor não tem mais pixels na mão —, então o canvas passa de conveniência a etapa obrigatória do
fluxo. Continua **não** sendo validação: quem controla o navegador pula o canvas e faz o POST direto ao
S3 com o presign. As garantias são as de T5.

**Respondendo às três dúvidas — sim, o navegador faz tudo isso, e faz melhor que o servidor:**

| Precisa | Como | Nota |
|---------|------|------|
| Dimensões | `const bmp = await createImageBitmap(file)` → `bmp.width/height` | **Lança** em arquivo que não é imagem decodificável; é a triagem grátis |
| Tipo real | o próprio `createImageBitmap` só aceita o que o navegador decodifica | melhor que `file.type`, que é só a extensão |
| Decode + resize | `OffscreenCanvas(192,192)` + `drawImage(bmp, sx,sy,sw,sh, 0,0,192,192)` | recorte quadrado pelo centro e resample numa chamada |
| Saída | `canvas.convertToBlob({type:'image/jpeg', quality:0.85})` | ~20 KB |

Três coisas que o canvas resolve de graça e o servidor não resolveria melhor:

1. **EXIF morre por construção.** O canvas guarda pixels, não metadado — a saída nunca tem `APP1`. É por
   isso que o confirm pode se dar o luxo de *recusar* EXIF em vez de removê-lo: o caminho normal nunca
   produz um.
2. **Orientação EXIF é aplicada ao rasterizar** (`createImageBitmap` respeita `imageOrientation:
   'from-image'`, que é o default). Foto de celular em retrato não chega de lado. O servidor só
   conseguiria isso lendo o EXIF que ele justamente não quer ler.
3. **Bomba de descompressão explode no navegador do atacante**, não no processo que serve as mesas.

`OffscreenCanvas` fora de Worker é suportado nos alvos do projeto; se der problema em algum, o fallback é
`document.createElement('canvas')` + `toBlob` — mesma API, uma linha.

**O POST vai para o S3, não pelo `client.ts`.** `client.ts:98-101` injeta `Authorization`; mandar esse
header para o S3 quebra a assinatura da policy (`Authorization` conflita com a auth do POST form).
Então: `client.post` nas duas rotas nossas, e `fetch(url, {method:'POST', body: formData})` **cru** para o
S3, sem interceptor e sem header manual — o `Content-Type` multipart o navegador põe sozinho. Ordem no
`FormData`: **todos os `fields` primeiro, o arquivo por último** — o S3 ignora campo que venha depois do
`file`.

**Nada no repo envia `FormData` hoje** — é o primeiro; não copiar padrão que não existe.

Erro de rede no meio deixa objeto em `up/` e nenhuma mudança de perfil: estado visível é "avatar
antigo", que é o certo. Retry é reabrir o seletor; o presign anterior expira em 120 s e a lifecycle
limpa o resto.

A mutation de `ProfileMenu.tsx:39-45` substitui o cache com o corpo da resposta
(`setQueryData(['player','me'], data)`), não invalida. O **confirm** devolve `playerResponse`, então cabe
na mesma mutation sem tocar em cache-busting: a URL nova já é outra chave.

### T10 — Avatar de 28 px no assento desktop

`globals.css:2812-2814` é `display: none` incondicional, com o comentário de `:2809-2810` explicando a
decisão ("assentos são cartões de nome"). Ligar com 28 px.

Sem foto o assento fica **idêntico ao de hoje**, porque o fallback é a inicial no mesmo espaço — é o que
torna a mudança segura de fazer sem redesenhar o assento. Registrar em `ui/DESIGN.md § 5`, que já tem
nota sobre o split desktop/retrato de `.seat-avatar`.

O anel vertical (`globals.css:5517-5522`) já dimensiona por `--seat-size` (38/46/52/34 px conforme
`:5471,5838,5860,6380`) — a imagem herda, nada a fazer. Estados cinza de `:5559-5563` (sitting_out /
disconnected / is-pending-name aplicam `grayscale(1) opacity(.65)`) passam a valer para a foto também,
o que é o comportamento desejado de graça.

## Fase 4 — Moderação

Foto de usuário na frente de outros jogadores é superfície de abuso, não detalhe. Mínimo viável, sem
painel de admin:

- `POST /v1.0/players/:id/avatar/report` — grava a denúncia, rate-limited por denunciante (`playerKey`
  de T5).
- `AvatarBlocked` no perfil: enquanto ligado, `AvatarURL()` (T4) devolve vazio, e **toda** superfície
  cai para iniciais sem código extra. Virar a flag é uma escrita no DynamoDB.

**Honestidade:** não há fila de revisão nem ferramenta de operador nesta fatia. É alavanca manual, e
isso é uma decisão de escopo, não um esquecimento. Se o volume crescer, o próximo passo é Rekognition
`DetectModerationLabels`, que encaixa entre os passos 2 e 3 do confirm de T5 — e o upload direto ao S3
**facilita** isso, porque `DetectModerationLabels` aceita `Image.S3Object` (bucket + chave), então o
objeto em `up/` é analisado onde já está, sem o servidor baixar byte nenhum. Reprovado = `DeleteObject` em
`up/` e nenhum `CopyObject`: a foto nunca chega a existir publicada.

## Fase 5 — Testes

Go (stdlib `testing`, sem testify — padrão do repo):

Com a interface S3 de T5 mockada, os testes 1–5 rodam sem rede.

1. `DecodeConfig` rejeita 5000×5000 **sem decodificar**. Assertar que a rejeição acontece antes da
   alocação (objeto em `up/` declarando dimensão grande com poucos bytes de payload) **e** que nenhum
   `CopyObject` foi chamado.
2. Rejeita GIF, WebP, SVG e HTML mesmo com o objeto em `up/` gravado com `Content-Type: image/jpeg`
   mentindo. É o teste que prova que o formato vem do decoder.
3. **EXIF é recusado**: JPEG com `APP1`/geolocalização em `up/` → confirm 422, nenhum `CopyObject`. É o
   teste que impede alguém "simplificar" a checagem depois, e o par natural do teste 12 (canvas nunca
   emite EXIF).
4. **`CopyObject` usa `MetadataDirective: REPLACE` e `ContentType: image/jpeg`.** Assertar sobre os
   parâmetros da chamada. É a única defesa contra corpo hostil servido na origem do app, e é uma linha
   que alguém apaga sem perceber.
5. **`version` diferente de `AvatarVersion + 1` é recusado** — inclusive uma versão *antiga* que ainda
   exista em `up/`. Sem este teste o confirm é um "publique qualquer coisa que eu tenha na quarentena".
6. Objeto ausente em `up/` → 404, não 500.
7. Versão incrementa; `up/` e `av/<v-1>` são deletados; **falha em qualquer dos dois deletes não falha o
   request**.
8. `SetAvatar` falhando **não** deixa perfil apontando para objeto inexistente (ordem do passo 5).
9. `AvatarURL` devolve vazio com `AvatarBlocked` — assertar nas quatro superfícies de T6, não só numa.
10. `avatar_url` presente no snapshot por assento.
11. **Criação de hand-share não copia avatar.** Assertar sobre o objeto gravado por `create`
    (`handshares.go:36-80`), não sobre a resposta de `public`.
12. Rate limit por jogador, não por IP: dois jogadores no mesmo IP não se limitam. Contado no
    `upload-url`, não no confirm.

UI (vitest, thresholds de `ui/vitest.config.ts`):

13. Fallback para iniciais sem URL e no `onError`.
14. Recorte no canvas produz 192×192 JPEG a partir de entrada retangular.
15. Saída do canvas **não tem EXIF** — o par do teste 3: JPEG de entrada com `APP1`, saída sem.
16. `FormData` põe todos os `fields` do presign **antes** do arquivo, e o POST ao S3 sai **sem** header
    `Authorization` (o interceptor de `client.ts:98-101` não pode alcançá-lo).
17. Assento sem foto renderiza igual ao de hoje (snapshot).
18. `profile/page.tsx` e `Seat.tsx` produzem **as mesmas** iniciais para o mesmo nome — é a regressão do
    bug `slice(0,2)` vs `initials()`.

CDK (`cdk/test/`): os quatro asserts listados em T2, mais — **o CSP de produção contém o endpoint do
bucket em `connect-src`**. Esse é o teste que evita a falha que só aparece em prod (T1b).

Manual: foto de celular em retrato, confirmar orientação; `GET /avatars/<id>/1.jpg` não cai no rewrite;
`GET /avatars/up/<id>/1.jpg` dá 403/404 (`originPath` faz a quarentena inalcançável).

## 📊 Resultado esperado

| Antes                                        | Depois                                              |
|----------------------------------------------|-----------------------------------------------------|
| Assento é nome + inicial                     | Foto de 192×192, fallback inicial no mesmo espaço   |
| `initials()` divergente entre showcase e mesa | Um `PlayerAvatar`, um `initials()`                 |
| Nenhum upload no `api/`                      | Presign + confirm, 5/hora/jogador, zero byte de imagem no processo do jogo |
| `.seat-avatar` só no anel de retrato         | Também no desktop, 28 px                            |
| Nenhum limite de body configurado            | 2 MB imposto pelo S3 na policy do presign           |
| —                                            | Quarentena `up/` inalcançável pela CDN, expira em 1 dia |

## 🔮 Fora deste plano

- **Mover a posse para `ctech-account`.** É o destino correto; o caminho de leitura foi desenhado para
  que a migração mexa só no writer.
- **Fila de revisão de moderação e ferramenta de operador.** `AvatarBlocked` é alavanca manual por ora.
- **Rekognition `DetectModerationLabels`.** Encaixa entre os passos 2 e 3 do confirm, sobre `Image.S3Object`.
- **Remover EXIF em vez de recusar.** Exigiria reescrever segmentos JPEG no servidor (~40 linhas de
  parser de marcador). Só vale se aparecer cliente legítimo que não passe pelo canvas.
- **Re-encode server-side.** Ficou de fora por decisão de arquitetura, não por dificuldade; as defesas
  que ele dava sozinho estão distribuídas na tabela da seção de decisão. Se um dia os bytes voltarem a
  passar pelo servidor, é `golang.org/x/image/draw` + `draw.CatmullRom` no confirm.
- **Múltiplos tamanhos / `srcset`.** 192×192 serve todas as superfícies atuais em 2x.
- **Avatar animado (GIF/WebP).** Rejeitado por decisão, não por dificuldade.
- **Avatar em `hand-shares`.** Rejeitado permanentemente: quebra a anonimização.
