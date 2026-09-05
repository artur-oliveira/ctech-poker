'use client';
import {Suspense, useState} from 'react';
import {useSearchParams} from 'next/navigation';
import {Users} from 'lucide-react';
import {TermsGate} from '@/components/TermsGate';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {FilterGroup} from '@/components/FilterGroup';
import {FriendCodeLookup} from '@/components/social/FriendCodeLookup';
import {PeopleList} from '@/components/social/PeopleList';
import {SocialInbox} from '@/components/social/SocialInbox';
import {
  listBlockedPlayers, listFriendRequests, listFriends, listRecentPlayers, listSocialInbox
} from '@/lib/api/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {useSocialList} from '@/lib/hooks/useSocialList';
import {SOCIAL_KEYS} from '@/lib/social';

type PeopleTab = 'friends' | 'requests' | 'recent' | 'blocked' | 'activity';
type RequestDirection = 'incoming' | 'outgoing';

const TABS = [
  {value: 'friends', label: 'Amigos'},
  {value: 'requests', label: 'Solicitações'},
  {value: 'recent', label: 'Recentes'},
  {value: 'blocked', label: 'Bloqueados'},
  {value: 'activity', label: 'Atividades'}
] as const;

const DIRECTIONS = [
  {value: 'incoming', label: 'Recebidas'},
  {value: 'outgoing', label: 'Enviadas'}
] as const;

function PeopleContent() {
  const actions = useSocialActions();
  // The unread badge links here with ?tab=activity: Atividades is the only
  // tab that marks inbox events read, so it is the only one that clears it.
  const params = useSearchParams();
  const [tab, setTab] = useState<PeopleTab>(() => {
    const requested = params.get('tab');
    return TABS.some(option => option.value === requested) ? requested as PeopleTab : 'friends';
  });
  const [direction, setDirection] = useState<RequestDirection>('incoming');

  // Every list is its own tab's read: the activity feed carries actor_name
  // server-side (#73), so nothing here loads friends or requests just to name
  // an event any more.
  const friends = useSocialList(SOCIAL_KEYS.friends, listFriends, tab === 'friends');
  const requests = useSocialList(SOCIAL_KEYS.requests(direction),
    cursor => listFriendRequests(direction, cursor), tab === 'requests');
  const recent = useSocialList(SOCIAL_KEYS.recent, listRecentPlayers, tab === 'recent');
  const blocked = useSocialList(SOCIAL_KEYS.blocked, listBlockedPlayers, tab === 'blocked');
  const inbox = useSocialList(SOCIAL_KEYS.inbox, listSocialInbox, tab === 'activity');

  return <AppPage authed current="people">
      <AppPageBody className="people-page">
        <AppPageHeader icon={Users} eyebrow="PESSOAS"
                       title="Seus amigos, solicitações e adversários recentes."
                       description="A amizade é sempre mútua. A presença aparece só entre amigos, e sua mesa só fica visível se você ativar isso no perfil — mesas privadas nunca aparecem."/>
        <FriendCodeLookup actions={actions}/>
        <FilterGroup label="Seções de pessoas" value={tab} options={TABS} onChangeAction={setTab}/>
        {tab === 'friends' && <PeopleList variant="friends" items={friends.items} isLoading={friends.isLoading}
                                          isError={friends.isError} isStale={friends.isStale}
                                          onRetryAction={friends.retry} hasNext={friends.hasNext}
                                          loadingMore={friends.loadingMore} onMoreAction={friends.loadMore}
                                          actions={actions}
                                          emptyTitle="Você ainda não tem amigos por aqui."
                                          emptyHint="Compartilhe seu código de amizade ou adicione alguém dos jogadores recentes."/>}
        {tab === 'requests' && <>
          <FilterGroup label="Direção das solicitações" value={direction} options={DIRECTIONS}
                       onChangeAction={setDirection}/>
          <PeopleList variant={direction} items={requests.items} isLoading={requests.isLoading}
                      isError={requests.isError} isStale={requests.isStale} onRetryAction={requests.retry}
                      hasNext={requests.hasNext} loadingMore={requests.loadingMore} onMoreAction={requests.loadMore}
                      actions={actions}
                      emptyTitle={direction === 'incoming' ? 'Nenhuma solicitação recebida.'
                        : 'Nenhuma solicitação enviada.'}
                      emptyHint={direction === 'incoming' ? 'Quando alguém te adicionar, aparece aqui.'
                        : 'Busque um código de amizade para enviar a primeira.'}/>
        </>}
        {tab === 'recent' && <PeopleList variant="recent" items={recent.items} isLoading={recent.isLoading}
                                         isError={recent.isError} isStale={recent.isStale}
                                         onRetryAction={recent.retry} hasNext={recent.hasNext}
                                         loadingMore={recent.loadingMore} onMoreAction={recent.loadMore}
                                         actions={actions}
                                         emptyTitle="Nenhum jogador recente ainda."
                                         emptyHint="Depois de uma mão, quem estava na mesa aparece aqui por 90 dias."/>}
        {tab === 'blocked' && <PeopleList variant="blocked" items={blocked.items} isLoading={blocked.isLoading}
                                          isError={blocked.isError} isStale={blocked.isStale}
                                          onRetryAction={blocked.retry} hasNext={blocked.hasNext}
                                          loadingMore={blocked.loadingMore} onMoreAction={blocked.loadMore}
                                          actions={actions}
                                          emptyTitle="Você não bloqueou ninguém."
                                          emptyHint="Bloquear esconde chat e reações, mas não muda as mesas públicas."/>}
        {tab === 'activity' && <SocialInbox events={inbox.items} isLoading={inbox.isLoading} isError={inbox.isError}
                                            hasNext={inbox.hasNext} loadingMore={inbox.loadingMore}
                                            onMoreAction={inbox.loadMore} onRetryAction={inbox.retry}
                                            actions={actions}/>}
      </AppPageBody>
    </AppPage>;
}

function PeopleFallback() {
  return <AppPage authed current="people">
    <AppPageBody className="people-page" aria-busy="true" aria-live="polite">
      <AppPageHeader icon={Users} eyebrow="PESSOAS"
                     title="Seus amigos, solicitações e adversários recentes."
                     description="Carregando suas conexões…"/>
      <div className="people-suspense-skeleton" aria-label="Carregando pessoas">
        <span className="skeleton"/><span className="skeleton"/><span className="skeleton"/>
      </div>
    </AppPageBody>
  </AppPage>;
}

export default function People() {
  return <TermsGate><Suspense fallback={<PeopleFallback/>}><PeopleContent/></Suspense></TermsGate>;
}
