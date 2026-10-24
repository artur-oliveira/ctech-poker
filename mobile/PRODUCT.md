# Product

## Platform

adaptive

## Users

Jogadores brasileiros de Texas Hold’em em Android e iOS. O uso principal é jogar
com amigos e oponentes públicos no celular, com sessões curtas ou prolongadas.

## Product Purpose

Cliente nativo do mesmo CTech Poker descrito em [../ui/PRODUCT.md](../ui/PRODUCT.md).
Preservar a identidade, as regras e a privacidade do produto, adaptando navegação,
áreas de toque, teclado e ciclo de vida ao aparelho. A paridade ainda está em
implementação; [docs/parity.md](docs/parity.md) registra os limites verificados.

## Brand Personality

Vívido, confiável e social. A direção já aprovada é **The Living Table**, documentada
em [../ui/DESIGN.md](../ui/DESIGN.md). Logo, IBM Plex e cores vêm do cliente web.
O mobile não tem uma marca ou uma paleta independente.

## Design Principles

- A mesma marca em login, navegação, mesa e ícone instalado.
- Vermelho identifica a ação principal; dourado identifica valor e turno.
- Material 3 organiza os controles; os tokens explícitos preservam a identidade.
- SafeArea, navegação do sistema, fontes ampliadas e redução de movimento continuam
  funcionando. IBM Plex é a fonte de marca solicitada em ambas as plataformas.
- Textos, formas e ícones acompanham estados: cor nunca é a única informação.

## Anti-references

Ícone genérico de baralho substituindo a logo, fundo azul-petróleo como identidade
global, tema dourado gerado automaticamente, fontes padrão substituindo IBM Plex,
ruído de cassino e promessas de homologação baseadas apenas em builds ou testes.
