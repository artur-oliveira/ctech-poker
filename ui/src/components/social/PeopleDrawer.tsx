'use client';
import {useState} from 'react';
import Link from 'next/link';
import {useRouter} from 'next/navigation';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger} from '@/components/ui/dialog';
import {PeopleList} from '@/components/social/PeopleList';
import {PeopleNavBadge} from '@/components/social/PeopleNavBadge';
import {listFriendRequests, listFriends, listRecentPlayers, listSocialInbox} from '@/lib/api/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {useSocialList} from '@/lib/hooks/useSocialList';
import {inviteActionable, nameResolver, SOCIAL_KEYS} from '@/lib/social';

const MAX_ONLINE_FRIENDS = 5;
const MAX_REQUESTS = 3;
const MAX_INVITES = 3;
const MAX_RECENT = 5;

/** Lobby quick panel: a side drawer on desktop, a bottom sheet on phones (the
 * geometry is CSS-only, see `.social-drawer`). It is a shortcut, not a second
 * implementation — every row reuses the same list and action components as
 * /people. Queries stay disabled until it is opened. */
export function PeopleDrawer() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const actions = useSocialActions();
  const friends = useSocialList(SOCIAL_KEYS.friends, listFriends, open);
  const requests = useSocialList(SOCIAL_KEYS.requests('incoming'),
    cursor => listFriendRequests('incoming', cursor), open);
  const recent = useSocialList(SOCIAL_KEYS.recent, listRecentPlayers, open);
  const inbox = useSocialList(SOCIAL_KEYS.inbox, listSocialInbox, open);

  const onlineFriends = friends.items
    .filter(friend => friend.presence && friend.presence !== 'offline')
    .slice(0, MAX_ONLINE_FRIENDS);
  const invites = inbox.items.filter(event => inviteActionable(event)).slice(0, MAX_INVITES);
  const nameOf = nameResolver(friends.items, requests.items);

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button type="button" variant="outline" className="app-nav-people-link"/>}>
      <Users aria-hidden="true"/> Pessoas<PeopleNavBadge/>
    </DialogTrigger>
    <DialogContent className="social-drawer">
      <DialogHeader>
        <DialogTitle>Pessoas</DialogTitle>
        <DialogDescription>Solicitações, convites, amigos online e quem jogou com você recentemente.</DialogDescription>
      </DialogHeader>
      <section aria-labelledby="people-drawer-requests">
        <h3 id="people-drawer-requests">Solicitações</h3>
        <PeopleList variant="incoming" items={requests.items.slice(0, MAX_REQUESTS)} isLoading={requests.isLoading}
                    isError={requests.isError} onRetryAction={requests.retry} actions={actions}
                    emptyTitle="Nenhuma solicitação pendente."/>
      </section>
      <section aria-labelledby="people-drawer-invites">
        <h3 id="people-drawer-invites">Convites de mesa</h3>
        {invites.length ? <ul className="people-list">
          {invites.map(invite => <li key={invite.event_id} className="people-row">
            <div className="people-row-identity">
              <b>{nameOf(invite.actor_id)} te convidou para uma mesa.</b>
            </div>
            <div className="people-row-actions">
              <Button type="button" disabled={actions.pending?.id === invite.event_id} onClick={async () => {
                if (await actions.run('accept-invite', invite.event_id)) {
                  setOpen(false);
                  router.push(`/table?id=${invite.room_id}`);
                }
              }}>Entrar</Button>
              <Button type="button" variant="ghost" disabled={actions.pending?.id === invite.event_id}
                      onClick={() => void actions.run('decline-invite', invite.event_id)}>Recusar</Button>
            </div>
          </li>)}
        </ul> : <div className="people-empty"><p>Nenhum convite ativo.</p></div>}
      </section>
      <section aria-labelledby="people-drawer-friends">
        <h3 id="people-drawer-friends">Amigos online</h3>
        <PeopleList variant="friends" items={onlineFriends} isLoading={friends.isLoading} isError={friends.isError}
                    onRetryAction={friends.retry} actions={actions}
                    emptyTitle="Nenhum amigo online agora."
                    emptyHint="A presença aparece só entre amigos."/>
      </section>
      <section aria-labelledby="people-drawer-recent">
        <h3 id="people-drawer-recent">Jogadores recentes</h3>
        <PeopleList variant="recent" items={recent.items.slice(0, MAX_RECENT)} isLoading={recent.isLoading}
                    isError={recent.isError} onRetryAction={recent.retry} actions={actions}
                    emptyTitle="Você ainda não jogou com ninguém."/>
      </section>
      <Link href="/people" className="social-actions-item">Ver todas as pessoas <ArrowRight aria-hidden="true"/></Link>
    </DialogContent>
  </Dialog>;
}
