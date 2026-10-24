# Revisão visual — 17/09/2026

Referências: feedback dos prints 01, 02, 04, 05, 06 e 38; componentes existentes
em `ui/src/components`, `ui/src/lib/cards.ts` e `ui/public/svgs`.

- Login: “Entrar com CTech Account”.
- Navegação: Lobby, Mãos, Pessoas, Loja e Perfil. Lucide LayoutGrid, History,
  Users, ShoppingBag e UserRound, correspondentes aos destinos do web.
- Lobby: escolha de blinds e formatos HEADS-UP, 6-MAX e FULL-RING. A disponibilidade
  usa `open_rooms` do servidor; a entrada continua em `join-or-create`, com os
  mesmos limites e garantias de idempotência. Mesa privada e convite ficam alinhados.
- Mãos: Histórico, Estatísticas, Conquistas, Ranking e Sessões; controles em uma
  linha, filtros horizontais e cartas ao lado do resultado. O filtro continua
  local às páginas carregadas, com indicação quando aplicado.
- Estatísticas: VPIP, PFR e 3-bet; valor, barra e amostra. A descrição completa
  abre no botão de informação. “Sem amostra” continua diferente de 0%.
- Conquistas: exemplos de cartas do catálogo web, estrelas e progresso. O servidor
  permanece a fonte de desbloqueios, metas e níveis; nenhuma conquista é inventada.
- Mesa: cartas SVG originais, assentos ao redor da borda com avatar/iniciais,
  feltro/borda de madeira, pote, cinco posições de board e progresso da rodada.
  Fold/Check/Pagar/Aumentar seguem o web. Texto ampliado ou altura insuficiente
  usa layout com rolagem; ações permanecem fora dele. Ações, snapshots, provas e
  privacidade continuam server-authoritative.

## Assets

`tool/sync-web-assets.mjs` exporta os ícones da instalação **lucide-react do web**
(ReactDOMServer, traço 1,8) e copia, sem editar, as 520 faces dos dez baralhos e o
verso vermelho. Lucide mantém sua licença ISC em `assets/icons/LICENSE`.
`flutter_svg` renderiza os assets do bundle, sem rede. Cartas inválidas/ocultas
usam o verso, sem tentar inferir rank ou naipe. O renderizador recorta apenas as
margens transparentes do SVG quadrado para manter a proporção física da carta.

## Modos

O produto não é descrito como exclusivo de sandbox. A interface usa “Fichas”,
como o web. `GameMode` separa o identificador da API, a nomenclatura e a formatação
(fichas inteiras versus centavos em BRL). Histórico, estatísticas, conquistas,
ranking e compartilhamento recebem o modo explicitamente e preservam esse escopo.
O cliente continua iniciando no modo Fichas; esta revisão **não habilita apostas
em dinheiro real**. Entrada real ainda depende da integração de carteira,
consentimento, taxas e habilitação do servidor, conforme a matriz de paridade.

## Evidência visual

`docs/screenshots/index.html` reúne a nova galeria. Os seis prints anteriores
citados no feedback estão em `docs/screenshots-before/`; a comparação lado a lado
fica em `docs/screenshots/comparison.html`. A numeração da galeria atual segue seu
manifesto; a mesa principal agora é 39.png.

Capturas de widgets Flutter com dados locais, sem barras de sistema, teclado ou
sessão autenticada. A mesa usa o componente de layout de produção com snapshot
fixo; não demonstra homologação multiplayer ou em aparelho.

## Validação final

- Análise estática sem problemas; 64 testes aprovados.
- Comparação byte a byte das 520 faces e do verso com os SVGs web.
- Histórico, estatísticas e conquistas em 320/390 dp com texto 1×/2×.
- Mesa em múltiplas dimensões, cards ocultos, snapshot de apresentação moderado,
  tamanho dos ícones e fluxos existentes de sessão/compra/replay.
- 50 prints finais exportados e APK Android debug compilado.

Ainda sem homologação autenticada em aparelhos ou iOS nesta etapa. Cosméticos,
animações e todos os estados da mesa web não são declarados idênticos por estas
capturas; esta é a revisão concreta das telas e elementos apontados no feedback.
