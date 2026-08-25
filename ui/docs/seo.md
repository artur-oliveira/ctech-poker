# SEO e rastreamento

Como o SPA é exportado estaticamente (`output: 'export'`), tudo que um crawler vê é gerado em build:
não há middleware, header dinâmico nem `noindex` em runtime.

## Origem canônica

`SITE_URL` (`src/lib/routeMetadata.ts`) é a única fonte da origem pública:
`NEXT_PUBLIC_APP_URL` com fallback `https://poker.aoctech.app`. Ela alimenta `metadataBase`
(`src/app/layout.tsx`), o `sitemap.xml` e o `robots.txt`. **Builds de produção precisam ter
`NEXT_PUBLIC_APP_URL` apontando para o domínio público** — caso contrário sitemap e robots saem
com a origem de desenvolvimento.

## Metadados por rota

Toda página é client component, então `metadata` vive no `layout.tsx` de cada rota e é montado por
`routeMetadata({title, description, path, image, index})`, que devolve canonical, Open Graph,
Twitter card e `robots`. `index` é **`false` por padrão**: uma rota só entra no índice quando é
legível sem sessão. Cada capítulo do `/guide` tem o próprio `layout.tsx` para não herdar o canonical
de `/guide` (canonical duplicado colapsaria os capítulos numa única URL).

Imagens OG ficam em `public/og/<slug>.webp` e cada slug usado precisa existir em
`OG_PREVIEWS` (`src/lib/ogPreviews.ts`) — há teste garantindo isso.

## robots.txt e sitemap.xml

- `src/app/robots.ts` → `out/robots.txt`: `Allow: /` com `Disallow` para as rotas com sessão
  (`PRIVATE_ROUTES`) e link absoluto para o sitemap.
- `src/app/sitemap.ts` → `out/sitemap.xml`: `INDEXABLE_ROUTES` (home, `/poker-rules`, `/guide` e
  seus sete capítulos), com prioridade e `changeFrequency`.

`/profile` continua indexável (vitrine pública), mas fora do sitemap: a URL depende de `?id=` do
jogador, então não há lista estática.

Ao criar uma rota pública nova: adicionar o `layout.tsx` com `index: true`, incluir o caminho em
`INDEXABLE_ROUTES` e capturar a imagem OG. Rota com sessão: manter o padrão (`index` omitido) e
acrescentar em `PRIVATE_ROUTES`. `src/app/crawlerSurface.test.ts` e `src/app/routeShells.test.tsx`
falham se as duas listas divergirem.
