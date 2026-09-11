# Paridade mobile — acompanhamento

Objetivo: preservar todos os fluxos do Poker Web em Android e iOS. Esta matriz
separa presença de implementação de homologação. Nenhum fluxo autenticado foi
homologado em produção ou em aparelho nesta sessão.

| Área | Implementado no cliente | Ainda necessário para paridade/homologação |
|---|---|---|
| Login | AppAuth/PKCE, refresh seguro e serializado, revogação | Registrar cliente público no Accounts e verificar implantação da PR Accounts #33 (integrada); testar ida/volta Android/iOS |
| Lobby | Buckets, buy-in sandbox, mesa privada, convite, retorno à sessão, aceite dos termos | Modo real sob gate e taxas; homologar a conferência de operações após reinício |
| Mesa | Protobuf, snapshots versionados, ações legais, aumento, pré-seleções, pausa, saída/cancelamento, chat/moderação, reações, recompra automática com consentimento, cartas, dois boards, resultados | Homologar auto-rebuy e handoff entre dispositivos; cenários de duração longa |
| Reconexão | Heartbeat, backoff, resync, lifecycle, não repetir aposta | Expiração de token durante socket, handoff e conflitos com web/CLI em integração |
| Revelações | Mostrar cartas individuais/ambas, pedidos ao vencedor, rabbit hunt com prova local | Homologar custos/consentimento e devolução por prova inválida; histórico pago |
| Histórico | Paginação, replay por ações com reprodução/velocidade/saltos por rodada, notas por street, coleções/revisão, filtros salvos, compartilhar com tipo/prazo/cartas e revogar, prova completa/parcial | Treinador do replay e filtros adicionais |
| Jornada | Estatísticas com amostras/explicações/estilo, conquistas com nomes editoriais, ranking global e pessoal paginado, sessões | Filtros adicionais e modo real sob gate |
| Perfil | Apelido, privacidade, foto, preferência de baralho/mesa, perfil público com ordem/visibilidade normalizadas, confronto independente, showcase e favoritos | Homologar showcase e apresentação visual de todos os cosméticos |
| Social | Amigos, pedidos recebidos/enviados e cancelamento, recentes, bloqueio, mute, denúncia, inbox, convite e perfil público | Todos os estados de privacidade, atualização de presença e acesso pela mesa |
| Loja | Catálogos, compras PIX/fichas, QR visual, histórico paginado, status/reembolso, chave estável no retry de compra | Homologar a conferência após morte do processo e todos os estados de erro/expiração em aparelhos |
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

A integração complementar no Accounts passou em `go test ./internal/domain/oauth/client ./internal/handler -race`; a API Poker passou em `go test ./... -race`. Os 22 testes Flutter passaram localmente; análise sem problemas. No commit 17570d2, o GitHub Actions aprovou quality, APK Android, simulador iOS e compilação iOS release sem assinatura (run 34619897621). A referência visual portrait passou também no Linux do Actions. Entrada/recompra agora preserva valor, consentimento e chave durante retries na mesma tela; a persistência após encerrar o processo foi adicionada na continuação abaixo. Handoff interrompe a reconexão automática da conexão antiga. A análise estática não apontou problemas. Respostas HTTP de sucesso malformadas são rejeitadas; falhas ao carregar filtros salvos impedem sobrescrever a lista desconhecida.

## Continuação após merge do Accounts #33

O perfil público respeita a ordem configurada, elimina seções duplicadas ou
desconhecidas e preserva a seção obrigatória de conquistas. O confronto possui
carregamento/erro/retry próprios: falhas nele não ocultam o perfil. Se a seção
está oculta ou o showcase é privado, o cliente não solicita o confronto.
Testes de widget cobrem esses comportamentos e a recuperação de erro transitório.

O carregador compartilhado também corrige o retry síncrono dentro de `setState`;
um teste verifica falha de rede seguida de carregamento bem-sucedido.

## Recuperação de operações e compartilhamento

Entrada, recompra, compra e reembolso salvam uma pendência em armazenamento seguro
antes do POST. Ela é particionada por API e usuário autenticado pelo endpoint `/me`.
Uma pendência bloqueia novas operações; o retry na mesma tela exige a mesma chave,
rota e conteúdo. Falhas no armazenamento impedem enviar o pedido. O app preserva
pendências em logout e reinício, sem guardar o convite privado nem tokens no registro.

O menu superior “Operações pendentes” abre a conferência via sessões/compras do
servidor. Não reenvia pedidos ao restaurar: as garantias do buy-in dependem do estado
da sessão e não permitem presumir replay seguro indefinidamente. Remover o aviso
exige confirmação do jogador após conferir; não cancela nem estorna a operação.
A persistência local não substitui uma conciliação automática no servidor.

O compartilhamento permite boa jogada/bad beat, prazo de 1/7/30 dias e opção explícita
de mostrar as próprias cartas (desligada inicialmente). O link pode ser selecionado,
copiado ou revogado na mesma tela; o perfil mantém a lista de links existentes.

Mudanças de identidade durante a preparação impedem enviar a operação com a
credencial da próxima conta; a pendência original é mantida para conferência.

## Jornada, replay e denúncias

O ranking pessoal usa `/leaderboard/me` (posição global, não índice da página),
com erro/retry independente do ranking comunitário. A lista conserva posições
entre páginas e abre o perfil público. Estatísticas exibem numeradores e amostras,
explicações de VPIP/PFR/3-bet e badges de estilo enviados pelo servidor; ausência
de oportunidade aparece como “Sem amostra”.

Replay: reprodução/pausa, velocidades 0,5×/1×/2×, início, passos e saltos por rodada.
A reprodução para ao chegar ao fim, ir ao segundo plano ou abrir notas/compartilhar.
Resultados finais e cartas dos adversários só aparecem no showdown/encerramento;
os frames não são reconstruídos a partir do resultado final. Treinador do replay
continua pendente.

Denúncias oferecem as seis categorias do Web, detalhes opcionais até 500 caracteres,
origem perfil/jogador recente/comportamento na mesa e contexto da mesa/mão quando
aplicável. Falhas preservam o mesmo conteúdo e chave no retry. Denúncia de mensagem
ou reação individual com evidência de action_id ainda está pendente.
