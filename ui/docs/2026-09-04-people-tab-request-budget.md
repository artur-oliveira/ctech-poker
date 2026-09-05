# /people: uma consulta por aba (#210)

## O que mudou

`GET /players/me/social/inbox` já resolve `actor_name` (e o avatar) de todo evento em um BatchGet
por página desde #73. A página `/people` ainda carregava, além do inbox:

- `friends` em **todas** as abas, e
- `requests` também na aba Atividades,

só para conseguir soletrar o nome do ator. Abrir `?tab=activity` custava três requests.

Agora cada aba carrega apenas a própria lista:

| aba | consulta |
| --- | --- |
| Amigos | `friends` |
| Solicitações | `requests(direction)` |
| Recentes | `recent` |
| Bloqueados | `blocked` |
| Atividades | `inbox` |

`SocialInbox` perdeu a prop `nameOf`: lê `event.actor_name` e, quando ele não vem (perfil removido),
cai no placeholder compartilhado `playerName()` — "Visitante". O helper `nameResolver` em
`lib/social.ts` foi removido junto com o comentário que ainda dizia que eventos trazem só o id do
ator.

`PeopleDrawer` continua carregando friends/requests/recent porque **renderiza** essas listas; ele
apenas deixou de repassar `nameOf`.

## Como validar

`src/app/(app)/people/page.test.tsx` cobre os dois lados do orçamento: abrir em Amigos não toca
inbox nem requests, e `?tab=activity` chama `listSocialInbox` uma vez e nenhuma das outras.
`socialComponents.test.tsx` cobre o placeholder de perfil removido.
