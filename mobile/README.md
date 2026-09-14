# CTech Poker Mobile

Cliente Flutter nativo para Android e iOS, em desenvolvimento. O motor permanece
na API Go e o transporte binário é gerado de `../proto/poker.proto`. A interface
não embute a aplicação web. Apenas o desafio Turnstile usa WebView; o login usa
AppAuth no navegador do sistema.

**Ainda não é uma substituição homologada do cliente web.** Consulte
[paridade e validação](docs/parity.md) antes de distribuir. Builds não demonstram
paridade funcional nem validação em aparelhos.

## Desenvolvimento

Flutter **3.47.3**, Dart 3.13.3. Dependências travadas em `pubspec.lock`.

```sh
flutter pub get --enforce-lockfile
./tool/generate-proto.sh # requer protoc no PATH
flutter analyze --no-pub
flutter test --no-pub
flutter run
```

Configuração por `--dart-define`:

- `POKER_API_URL`: `https://poker-api.aoctech.app`
- `ACCOUNTS_API_URL`: `https://accounts-api.aoctech.app`
- `TURNSTILE_SITE_KEY`: chave pública do Turnstile, necessária para os desafios.

Não inclua client secret, token de usuário ou credenciais AWS no build.

## Autenticação e integração

O cliente público de primeira parte é **poker-mobile**, callback exato
`app.aoctech.poker:/oauth/callback`. Android e iOS registram esse esquema. AppAuth
controla state e PKCE; a troca HTTP captura apenas o cookie de refresh cujo nome
é `ctech_rt_` mais os primeiros 16 caracteres do SHA-256 do client ID. O refresh
é armazenado por `flutter_secure_storage`; o access token fica em memória.
Renovações concorrentes compartilham uma operação. Falhas transitórias preservam
a credencial; `invalid_grant` a remove. Logout bloqueia novas renovações enquanto revoga a cadeia deste cliente.
O target iOS declara Keychain nos entitlements. Android preserva a afinidade
padrão da Activity para permitir o retorno do navegador AppAuth.

**Dependência de integração pendente:** registrar `poker-mobile` no Accounts com
os 12 scopes `poker:*:read` e `openid profile`, audience da API Poker e a callback
acima. A alteração complementar em `../ctech-account` permite o esquema nativo com
validação restrita e testes. Ela precisa ser integrada/deployada antes do cadastro
do cliente. Uma alternativa futura são App Links/Universal Links associados. Não reutilizar o client
ID do navegador, registrar um segredo no aplicativo ou ampliar redirects de
clientes existentes. Nenhum cadastro de produção foi alterado por esta mudança.
A API Poker já inclui `poker-mobile` na allowlist, sempre exigindo `sid`.

## GitHub Actions sem Mac local

`../.github/workflows/mobile.yml` executa análise, testes e comparação do protobuf
gerado. Após isso:

- Linux gera APK **debug**, instalável para desenvolvimento, em artifact.
- macOS gera `.app` para simulador, arquivado preservando links/metadados.
- macOS também verifica compilação release para dispositivo com `--no-codesign`.

O artifact de simulador não é um IPA instalável em iPhone. Para TestFlight/App Store,
será necessário Apple Developer, App ID `app.aoctech.poker`, certificado/distribution
profile e configuração protegida de assinatura no GitHub. Para Google Play,
configurar keystore de release e gerar AAB. O projeto gerado ainda usa assinatura
debug no target Android release; não publicar esse binário. O workflow atual não
publica em lojas nem faz deploy da API. A primeira execução remota (34619897621, commit 17570d2) aprovou os dois
platform targets, incluindo compilação iOS para dispositivo sem assinatura.
Cada atualização deve passar novamente pelo workflow antes de usar seus artifacts.

## Estrutura

- `lib/core`: sessão, HTTP, gateways de lobby/mesa, preferências e provas locais.
- `lib/generated`: saída do protoc, nunca editar manualmente.
- `lib/features`: navegação, lobby, mesa, biblioteca, comunidade, loja e plugins.
- `test`: privacidade, layouts, provas, refresh, idempotência e protocolo.

O estado da mesa pertence ao servidor. Apostas levam UUID, versão de snapshot e
hand ID. Ações não são enfileiradas durante desconexão. A confirmação de um aumento
é descartada se a versão mudou enquanto o seletor estava aberto. Ao retornar do
segundo plano, uma nova sincronização precede qualquer aposta. Histórico usa
milissegundos e paginação do servidor; cartas `back` nunca são reconstruídas.

## Operações sem confirmação

Use “Operações pendentes” no menu superior para conferir sessões e compras após
uma interrupção. Entradas, recompras, compras e reembolsos persistem uma pendência
por conta antes de enviar. O app não repete operações automaticamente ao reiniciar.
A retirada manual do aviso apenas libera novas operações; confira o resultado e o
saldo antes de confirmar. Veja os limites em [paridade](docs/parity.md).

## Identidade visual

[PRODUCT.md](PRODUCT.md) e [DESIGN.md](DESIGN.md) registram a adaptação da identidade
já existente em `../ui/`, documentada com `impeccable document`. A logo vem de
`../ui/public/svgs/logo.svg`; cores e tipografia seguem o CSS e layout do Web.
`PokerTheme.dark` é o único tema da aplicação e dos testes visuais. Não substituir
os tokens por um tema gerado automaticamente.

IBM Plex Sans/Mono estão no bundle em `assets/fonts`, com licença OFL e origem
fixada no DESIGN.md. Para regenerar a logo e os recursos Android/iOS a partir do
SVG original (requer as dependências de `ui/` já instaladas):

```sh
node tool/generate-brand.mjs
flutter test --no-pub test/brand_test.dart test/table_golden_test.dart
```

Mudanças intencionais de aparência exigem inspeção das imagens antes de aceitar
novos goldens. Veja a [conferência visual](docs/brand-review.md). Android/iOS ainda
precisam de validação em aparelhos; testes de widget não validam o launcher do SO.
