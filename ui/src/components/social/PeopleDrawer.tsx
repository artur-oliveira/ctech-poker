'use client';
import {useState} from 'react';
import Link from 'next/link';
import {ArrowRight, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger} from '@/components/ui/dialog';
import {PeopleList} from '@/components/social/PeopleList';
import {PeopleNavBadge} from '@/components/social/PeopleNavBadge';
import {SocialInbox} from '@/components/social/SocialInbox';
import {listFriendRequests, listFriends, listRecentPlayers, listSocialInbox} from '@/lib/api/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {useSocialList} from '@/lib/hooks/useSocialList';
import {nameResolver, SOCIAL_KEYS} from '@/lib/social';

const MAX_ONLINE_FRIENDS = 5;
const MAX_REQUESTS = 3;
const MAX_RECENT = 5;

/** Lobby quick panel: a side drawer on desktop, a bottom sheet on phones (the
 * geometry is CSS-only, see `.social-drawer`). It is a shortcut, not a second
 * implementation — every row reuses the same list and action components as
 * /people. Queries stay disabled until it is opened. */
export function PeopleDrawer() {
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
  const nameOf = nameResolver(friends.items, requests.items);

  return <Dialog open={open} onOpenChange={setOpen} modal={false}>
    <DialogTrigger render={<Button type="button" size="lg" variant="outline" className="app-nav-people-link"/>}>
      <Users aria-hidden="true"/> Pessoas<PeopleNavBadge/>
    </DialogTrigger>
    <DialogContent backdrop={false} className="social-drawer translate-none p-0">
      <DialogHeader>
        <DialogTitle>Pessoas</DialogTitle>
        <DialogDescription>Solicitações, convites, amigos online e quem jogou com você recentemente.</DialogDescription>
      </DialogHeader>
      <div className="social-drawer-body">
        <section aria-labelledby="people-drawer-requests">
          <h3 id="people-drawer-requests">Solicitações</h3>
          <PeopleList variant="incoming" items={requests.items.slice(0, MAX_REQUESTS)} isLoading={requests.isLoading}
                      isError={requests.isError} onRetryAction={requests.retry} actions={actions}
                      emptyTitle="Nenhuma solicitação pendente."/>
        </section>
        <section aria-labelledby="people-drawer-activity">
          <h3 id="people-drawer-activity">Atividade</h3>
          {/* The unread badge on the trigger counts inbox events, so the drawer
              has to render the same feed — otherwise the only way to clear the
              badge is navigating to /people and opening the Atividades tab. */}
          <SocialInbox events={inbox.items} isLoading={inbox.isLoading} isError={inbox.isError}
                       onRetryAction={inbox.retry} actions={actions} nameOf={nameOf}
                       onNavigateAction={() => setOpen(false)}/>
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
      </div>
    </DialogContent>
  </Dialog>;
}
