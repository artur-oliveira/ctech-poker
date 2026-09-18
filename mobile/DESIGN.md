---
name: CTech Poker Mobile
description: Adaptação nativa da identidade existente do CTech Poker Web.
colors:
  brand: "#af2a2f"
  brand-active: "#d9464d"
  wine: "#5b1218"
  ink: "#120d0e"
  paper: "#f6f0e7"
  gold: "#e6b85c"
  gold-ink: "#30230a"
  felt: "#0d5b45"
  felt-light: "#18765b"
  felt-dark: "#084b38"
  felt-text: "#e3f1ea"
  felt-value: "#f3e9c9"
  rail: "#7c4d2f"
  seat: "#161011"
  control: "#211416"
  control-hover: "#3e3133"
  muted: "#ad9fa0"
  secondary-text: "#cbbfc0"
  success: "#48c98c"
  danger: "#dc2626"
  danger-text: "#ef4444"
  danger-soft: "#f5b0b3"
  error-surface: "#3b0b0e"
  focus: "#ed777c"
  on-brand: "#ffffff"
  border: "#ffffff24"
  seat-border: "#ffffff26"
typography:
  title:
    fontFamily: "IBM Plex Sans"
    fontSize: "24sp"
    fontWeight: 700
  body:
    fontFamily: "IBM Plex Sans"
    fontSize: "14sp"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "IBM Plex Sans"
    fontSize: "14sp"
    fontWeight: 600
  readout:
    fontFamily: "IBM Plex Mono"
    fontWeight: 600
rounded:
  control: "12dp"
  seat: "14dp"
  panel: "16dp"
spacing:
  sm: "8dp"
  md: "16dp"
  lg: "24dp"
components:
  button-primary:
    backgroundColor: "{colors.brand}"
    textColor: "{colors.on-brand}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    height: "52dp minimum"
  action-call:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.wine}"
    rounded: "{rounded.control}"
  input:
    backgroundColor: "{colors.control}"
    textColor: "{colors.paper}"
    rounded: "{rounded.control}"
  dialog:
    backgroundColor: "{colors.control}"
    textColor: "{colors.paper}"
    rounded: "{rounded.panel}"
---

# Design System: CTech Poker Mobile

## Overview

**Creative North Star: “The Living Table”**, preservada de `../ui/DESIGN.md`.
A sala escura deixa a mesa, as pessoas e a decisão atual em primeiro plano.
O mobile usa a mesma identidade vívida, confiável e social do Web.

Este documento foi extraído do código existente com o fluxo `impeccable document`.
A voz e os nomes já estavam definidos em `../ui/PRODUCT.md` e `../ui/DESIGN.md`;
esta adaptação não redefine a direção criativa.

Fontes de verdade, em ordem: `../ui/public/svgs/logo.svg`, tokens de
`../ui/src/app/base.css`, variantes de `../ui/src/app/renderer.css`, fontes em
`../ui/src/app/layout.tsx`. O mapeamento nativo está em `lib/core/design.dart`
(`PokerColors`/`PokerTheme`) e `lib/core/preferences.dart` (temas de mesa).
Os testes comparam os principais tokens e o SVG entre os dois clientes.

## Colors

Os valores normativos estão no frontmatter. Correspondência Web → Flutter:

| Web | Flutter | Uso |
|---|---|---|
| `--brand` | `PokerColors.brand` | Botão principal e seleção da navegação |
| `--ink` | `PokerColors.ink` | Fundo global e app bar |
| `--wine` | `PokerColors.wine` | Estado selecionado, verso de carta e tinta em botão claro |
| `--surface-seat` | `PokerColors.seat` | Assentos, navegação e cards |
| `--surface-control` | `PokerColors.control` | Inputs, diálogos e sheets |
| `--paper` | `PokerColors.paper` | Cartas, texto principal, Pagar/Passar |
| `--gold` | `PokerColors.gold` | Fichas em assentos, recompensas e turno |
| `--seat-stack-ink` | `PokerAppearance.feltValueColor` | Valores sobre feltro |
| `--table-rail` | `PokerAppearance.railColor` | Borda física da mesa |
| `--focus-ring` | `PokerColors.focus` | Foco, progresso, slider e cursor |
| `--muted`, `--text-secondary` | `muted`, `secondaryText` | Textos auxiliares |

**Três materiais:** feltro significa jogo, papel significa cartas ou ação clara,
dourado significa valor/tempo/conquista. Acentos vermelhos não são usados como
texto pequeno sobre fundo escuro. Os botões vermelhos têm texto branco; abas e
links usam texto claro. Os pares de texto são verificados com contraste ≥4,5:1.

As variantes classic/midnight/burgundy/ocean preservam os gradientes de feltro
já correspondentes ao Web e agora também a cor de borda. O valor sobre feltro
usa a cor mais clara em classic/ocean e dourado em midnight/burgundy, como no Web.
`ColorScheme` define as superfícies explicitamente; não usar `fromSeed` ou cores
dinâmicas do sistema que substituam a paleta solicitada.

## Typography

IBM Plex Sans 400/500/600/700 em títulos, corpo e controles. IBM Plex Mono
400/600/700 em valores de pote e fichas. Fontes TTF completas, locais, declaradas
em `pubspec.yaml`, obtidas de [IBM/plex](https://github.com/IBM/plex) no commit
`bf260093582f04622aacc1e9f9ca604d7ccd0c42`; licença OFL em `assets/fonts/OFL.txt`.
Não há download de fonte em runtime.

As escalas Material continuam ampliáveis pelo sistema. O mobile não copia os
headlines de marketing de 82px: aplica os papéis de título/corpo/label acima.
Cartas mantêm tamanho tipográfico ligado à geometria da carta e descrição
semântica; os outros textos respeitam `TextScaler`.

## Elevation

Superfícies usam as camadas escuras da marca, sem a tonalização automática que
alterava as cores. Cards e app bar ficam planos; sheets e diálogos usam as
transições nativas existentes. A borda da mesa representa a madeira do Web.
Não acrescentar brilho ou sombra decorativa a botões, cards ou à mesa.

## Components

### Logo e ícone instalado

`PokerLogo` usa `assets/brand/logo.png` a 512px, rasterizado diretamente do SVG
original, preservando os dois traços, cores e proporção. O SVG é copiado sem
alterações para rastreabilidade. Não redesenhar o monograma em um `CustomPainter`
nem substituí-lo por `Icons.style`.

Login: 76dp; cabeçalho: 32dp; buy-in: 64dp; centro da mesa: 26dp. O asset tem rótulo
acessível “CTech Poker”. `tool/generate-brand.mjs` produz os PNGs da interface,
launch screens, todos os tamanhos iOS e ícones Android a partir do SVG do Web.
Ícones iOS são opacos, com fundo vermelho completo; o sistema aplica a máscara.
Android usa adaptive icon com margem segura, fundo vermelho e versão monocromática.
A abertura usa o fundo da sala nos dois sistemas, inclusive com aparência clara
do aparelho. A apresentação real pelo launcher ainda requer validação em aparelho.

### Controles e navegação

Botões usam raio de controle e área mínima de toque de 48dp (52dp nos filled).
Pagar/Passar são claros, Aumentar é vermelho, Desistir é tonal escuro. Estados de
carregamento/desabilitado e bloqueios de aposta existentes são preservados.
Inputs têm fundo de controle, texto legível e borda de foco clara. Cards/sheets/
diálogos usam raio de painel. Navegação mantém os cinco destinos existentes,
indicador vermelho e ícone branco selecionado.

### Verificação e limites

Referências visuais em `test/goldens/brand_*.png` e `table_portrait.png` usam o
**tema de produção** e fontes do bundle. Os testes anteriores usavam um tema de
exemplo independente; não eram evidência da identidade correta do aplicativo.
Testes incluem fontes ampliadas, cores confrontadas com o CSS e contraste nos
quatro feltros. Atualizar goldens exige inspeção visual, não apenas aprovação
mecânica. Paridade de ilustração das cartas, cosméticos e geometria do renderer
Web continua separada de fidelidade de logo/paleta/tipografia.

## Do's and Don'ts

- Reutilizar os tokens e a logo reais de `ui/` antes de criar uma tela.
- Preservar SafeArea, redução de movimento, escala de texto e navegação do sistema.
- Usar os mesmos assets/fontes/tema nos testes visuais e no aplicativo.
- Não usar azul-petróleo ou dourado como identidade global alternativa.
- Não alterar a fonte por uma escolha padrão de Material/iOS.
- Não declarar paridade visual de todas as telas a partir de um golden da mesa.

## Revisão de consistência com o web — 17/09/2026

Esta revisão substitui as descrições anteriores de ícones Material e cartas
pintadas no Flutter: os ícones explícitos agora são SVGs Lucide da mesma instalação
do web, com 1,8 de traço e tamanho herdado do controle. Voltar/fechar também usam
Lucide pelo tema. Cartas usam **os mesmos 520 SVGs e verso vermelho**, incluindo
os dez baralhos; não há `SuitPainter` ou arte de carta independente.

A navegação passa a Lobby / Mãos / Pessoas / Loja / Perfil, com rótulos de uma
linha e seleção em vinho. Lobby usa os nomes e a escolha de blinds/tamanho do web.
Estatísticas movem a explicação longa para informação sob demanda. Conquistas
usam a arte do catálogo. Na mesa, os assentos ficam na borda, com avatar/iniciais,
o board possui cinco posições e a rodada é marcada por pontos como no renderer
web. Tamanhos insuficientes ou fonte ampliada continuam com alternativa rolável.

A nomenclatura de modo é **Fichas / Dinheiro real**. Não usar “fichas recreativas”,
“Escolha o seu ritmo” ou “Minha jornada”. A ativação financeira não é consequência
da revisão visual. Detalhes e fontes em [docs/web-parity-review.md](docs/web-parity-review.md).
