# Paridade mobile — acompanhamento

Objetivo: preservar todos os fluxos do Poker Web em Android e iOS. Esta matriz
separa presença de implementação de homologação. Nenhum fluxo autenticado foi
homologado em produção ou em aparelho nesta sessão.

| Área | Implementado no cliente | Ainda necessário para paridade/homologação |
|---|---|---|
| Login | AppAuth/PKCE, refresh seguro e serializado, revogação | Registrar cliente e viabilizar redirect nativo no Accounts; testar ida/volta Android/iOS |
| Lobby | Buckets, buy-in sandbox, mesa privada, convite, retorno à sessão, aceite dos termos | Modo real sob gate e taxas; recuperar chaves de operações incertas após reinício |
| Mesa | Protobuf, snapshots versionados, ações legais, aumento, pré-seleções, pausa, saída/cancelamento, chat/moderação, reações, recompra automática com consentimento, cartas, dois boards, resultados | Homologar auto-rebuy e handoff entre dispositivos; cenários de duração longa |
| Reconexão | Heartbeat, backoff, resync, lifecycle, não repetir aposta | Expiração de token durante socket, handoff e conflitos com web/CLI em integração |
| Revelações | Mostrar cartas individuais/ambas, pedidos ao vencedor, rabbit hunt com prova local | Homologar custos/consentimento e devolução por prova inválida; histórico pago |
| Histórico | Paginação, replay por ações, notas por street, coleções/revisão, filtros salvos, compartilhar/revogar, prova completa/parcial | Controles completos do replay e filtros adicionais |
| Jornada | Estatísticas, conquistas com nomes editoriais, ranking, sessões | Ranking pessoal, todos os filtros e modos |
| Perfil | Apelido, privacidade, foto, preferência de baralho/mesa, perfil público/confronto, showcase e favoritos | Homologar showcase e apresentação visual de todos os cosméticos |
| Social | Amigos, pedidos recebidos/enviados e cancelamento, recentes, bloqueio, mute, denúncia, inbox, convite e perfil público | Todos os estados de privacidade, atualização de presença e acesso pela mesa |
| Loja | Catálogos, compras PIX/fichas, QR visual, histórico paginado, status/reembolso, chave estável no retry de compra | Persistência após morte do processo, todos os estados de erro/expiração em aparelhos |
| Recursos nativos | Foto, microfone com confirmação, preferências, entrada para desafio Turnstile | Validar permissões, TTS, sons, lembretes, treinador, domínio Turnstile em Android/iOS |
| UI | Material 3 escuro, navegação inferior, mesa portrait/landscape com disposição ao redor do board, SafeArea, cartas acessíveis | Revisão visual e usabilidade de todas as telas; disposição de nove jogadores em todos os tamanhos reais; referências visuais das outras telas |
| Distribuição | Workflow Android + macOS, APK debug, simulador, compile iOS sem assinatura | Assinatura Android/iOS; TestFlight; validação em aparelhos |

## Evidências automatizadas

- Layout da mesa com nove assentos em 320×568, 390×844, 844×390 e 768×1024,
  escalas de texto 1×/2×. É teste de ausência de overflow, não aprovação visual.
- Cartas ocultas não expõem rank/suit e possuem rótulo acessível.
- Provas SHA-256/HMAC contra fixture independente; rejeição de provas incompletas
  ou adulteradas. A fixture não contém dados de uma partida real.
- Sessão: refresh concorrente, persistência do cookie, erro transitório versus
  invalid_grant, retry de 401 preservando o corpo e chave da operação.
- Socket: primeiro frame de auth, bloqueio antes do snapshot, rejeição de estado
  antigo, limites de aumento, clique duplicado, reconexão sem replay de aposta.

Não marcar esta matriz como concluída com base apenas em telas ou endpoints
presentes. Os testes de ponta a ponta devem cobrir entrada/saída com saldo,
múltiplos jogadores e clientes, rede instável, bloqueio do aparelho, token expirado,
compra seguida de reembolso e cartas privadas.

A integração complementar no Accounts passou em `go test ./internal/domain/oauth/client ./internal/handler -race`; a API Poker passou em `go test ./... -race`. Os 22 testes Flutter passaram localmente; análise sem problemas. No commit 17570d2, o GitHub Actions aprovou quality, APK Android, simulador iOS e compilação iOS release sem assinatura (run 34619897621). A referência visual portrait passou também no Linux do Actions. Entrada/recompra agora preserva valor, consentimento e chave durante retries na mesma tela; a persistência após encerrar o processo continua pendente. Handoff interrompe a reconexão automática da conexão antiga. A análise estática não apontou problemas. Respostas HTTP de sucesso malformadas são rejeitadas; falhas ao carregar filtros salvos impedem sobrescrever a lista desconhecida.
