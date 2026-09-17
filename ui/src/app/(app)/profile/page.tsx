'use client';
import {Suspense} from 'react';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ArrowLeft, Eye, Lock, Pencil, Sparkles, Swords, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import type {MatchupStats} from '@/lib/api/player';
import {getMatchupStats, getProfileShowcase, handEndedAtMs, normalizeShowcaseLayout} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';
import {useOptionalSession} from "@/lib/auth/session";
import {getViewerId} from '@/lib/utils';
import {AppPage} from '@/components/AppPageChrome';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {PlaystyleBadges} from '@/components/PlaystyleBadges';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import {PlayerActionsMenu} from '@/components/social/PlayerActionsMenu';
import {ProfileMilestones} from '@/components/ProfileMilestones';
import {getRelationship} from '@/lib/api/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {SOCIAL_KEYS} from '@/lib/social';

function statusOf(error: unknown): number | undefined {
  return (error as {status?: number} | null | undefined)?.status;
}

// Loading and every failure branch keep this landmark + heading so the page
// never renders without an h1 (CLAUDE.md: a real h1 survives every state).
function ShowcaseShell({children}: {children: React.ReactNode}) {
  return <section className="profile-showcase shell">{children}</section>;
}

function ShowcaseError({status}: {status?: number}) {
  const isPrivate = status === 403;
  return <div className="lobby-empty">
    {isPrivate ? <Lock aria-hidden="true"/> : <Sparkles aria-hidden="true"/>}
    <h1>{isPrivate ? 'Vitrine privada' : 'Vitrine indisponível'}</h1>
    <p>{isPrivate
      ? 'Este jogador mantém a vitrine privada. Só ele decide quando torná-la pública.'
      : status === 404
        ? 'Este perfil não existe ou foi removido.'
        : 'Este perfil não existe ou não foi encontrado.'}</p>
    <Button render={<Link href="/lobby"/>}>Ir para o Lobby</Button>
  </div>;
}

/** The owner's own strip above their showcase. Outside preview it offers the
 * two things only the owner can do; inside preview it is the only thing on
 * screen that a visitor would not see, and it says so. */
function OwnerBar({playerID, preview}: {playerID: string; preview: boolean}) {
  const visitorHref = `/profile?id=${encodeURIComponent(playerID)}&preview=1`;
  return <div className="profile-owner-bar" data-preview={preview || undefined}>
    <p>{preview
      ? 'Pré-visualização: é assim que um visitante vê seu perfil.'
      : 'Esta é a sua vitrine. Só você vê esta faixa.'}</p>
    <div className="profile-owner-bar-actions">
      {preview
        ? <Button variant="outline" render={<Link href={`/profile?id=${encodeURIComponent(playerID)}`}/>}>
          <ArrowLeft aria-hidden="true"/> Sair da pré-visualização
        </Button>
        : <Button variant="outline" render={<Link href={visitorHref}/>}>
          <Eye aria-hidden="true"/> Ver como visitante
        </Button>}
      <Button render={<Link href="/player-profile"/>}><Pencil aria-hidden="true"/> Editar perfil</Button>
    </div>
  </div>;
}

/** The owner asking for a showcase the server will not serve: the only reason
 * their own showcase 404s is that it is not public. A visitor's copy would be
 * a lie here, so the owner gets the real cause and the way out. */
function OwnShowcasePrivate({preview}: {preview: boolean}) {
  return <div className="lobby-empty">
    <Lock aria-hidden="true"/>
    <h1>Vitrine privada</h1>
    <p>{preview
      ? 'Enquanto ela estiver privada, um visitante não abre este link, e é só isso que ele vê.'
      : 'Ninguém além de você abre este link. Deixe a vitrine pública para poder compartilhá-la.'}</p>
    <Button render={<Link href="/player-profile"/>}>Editar perfil</Button>
  </div>;
}

function ProfileContent() {
  const params = useSearchParams();
  const playerID = params.get('id') || '';
  // The owner's own link used to dead-end on an "esta é a sua vitrine" panel,
  // so nobody could see what they were publishing. It now renders the real
  // showcase, and `preview` drops the owner-only affordances so what is left
  // on screen is exactly the visitor's view.
  const preview = params.get('preview') === '1';
  const {authed} = useOptionalSession();
  const viewerID = getViewerId();
  const isOwnProfile = Boolean(playerID) && playerID === viewerID;
  // Enabled for the viewer's own id too: the endpoint has no viewer check, so
  // it answers with the same payload a visitor gets, or 404s for exactly one
  // reason — the showcase is not public.
  const showcase = useQuery({
    queryKey: ['profile-showcase', playerID],
    queryFn: () => getProfileShowcase(playerID),
    enabled: Boolean(playerID)
  });
  const matchup = useQuery({
    queryKey: ['profile-matchup', playerID],
    queryFn: () => getMatchupStats(playerID),
    // The matchup endpoint 400s for the viewer's own id — disable it there
    // rather than relying on hands_together to hide the section after a throw.
    enabled: authed && Boolean(playerID) && !isOwnProfile
  });
  const socialActions = useSocialActions();
  // Fails (400) for the viewer's own profile, which is exactly when no safety
  // or friendship action should be offered — so an absent result hides the menu.
  const relationship = useQuery({
    queryKey: SOCIAL_KEYS.relationship(playerID),
    queryFn: () => getRelationship(playerID),
    enabled: authed && Boolean(playerID) && !isOwnProfile
  });

  return <AppPage authed={authed} footer={false}>
    <ShowcaseShell>
      {isOwnProfile && <OwnerBar playerID={playerID} preview={preview}/>}
      {showcase.isLoading ?
        <>
          <h1 className="sr-only">Vitrine do jogador</h1>
          <LoadingRegion label="Carregando vitrine do jogador…" className="skeleton-panel profile-showcase-skeleton">
            <Skeleton style={{height: '68px', width: '68px', borderRadius: '50%'}}/>
            <Skeleton style={{height: '26px', width: 'min(260px, 70%)'}}/>
            <Skeleton style={{height: '150px'}}/>
            <Skeleton style={{height: '150px'}}/>
          </LoadingRegion>
        </> :
        showcase.isError || !showcase.data
          ? isOwnProfile ? <OwnShowcasePrivate preview={preview}/> : <ShowcaseError status={statusOf(showcase.error)}/>
          : <>
          <header>
            <PlayerAvatar className="profile-showcase-avatar" name={showcase.data.name}
                          avatarUrl={showcase.data.avatar_url} size={68}/>
            <div>
              <small>VITRINE DO JOGADOR</small>
              <h1>{showcase.data.name || 'Jogador'}</h1>
              <ProfileMilestones memberSince={showcase.data.member_since}
                                 milestones={showcase.data.milestones}/>
              {showcase.data.playstyle?.length
                ? <PlaystyleBadges badges={showcase.data.playstyle} className="profile-playstyle-badges"/>
                : null}
            </div>
            {relationship.data && <PlayerActionsMenu target={relationship.data} actions={socialActions}
                                                     surface="profile"/>}
          </header>
          {(() => {
            const layout = normalizeShowcaseLayout(showcase.data.showcase_layout);
            const visible = layout.order.filter(id => !layout.hidden.includes(id));
            // Unreachable today (achievements can never be hidden — see
            // normalizeShowcaseLayout), kept because a corrupted/future layout
            // must never render a silent blank showcase.
            if (visible.length === 0) return <p className="profile-showcase-empty">Nenhuma seção visível nesta vitrine.</p>;
            return <div className="profile-showcase-content">
              {visible.map(id => {
                if (id === 'achievements') return <section key={id}>
                  <h2><Trophy aria-hidden="true"/> Conquistas em Destaque</h2>
                  {showcase.data.featured_achievements.length ? <div className="profile-featured-list">
                    {showcase.data.featured_achievements.map(item => <article key={item.key}>
                      <Sparkles aria-hidden="true"/>
                      <div>
                        <b>{achievementLabel(item.key)}</b>
                        <span>{item.count.toLocaleString('pt-BR')} registradas</span>
                      </div>
                    </article>)}
                  </div> : <p className="profile-showcase-empty">Nenhuma conquista selecionada para exibição.</p>}
                </section>;
                if (id === 'best_hand') return <section key={id}>
                  <h2>Melhor Vitória Recente</h2>
                  {showcase.data.best_hand ? <article className="profile-best-hand">
                    <div className="profile-best-hand-cards">
                      {(showcase.data.best_hand.hole_cards || []).map((card, index) =>
                        <PlayingCard key={`${card}-${index}`} card={card} index={index} size="hole"/>)}
                    </div>
                    <b>+{showcase.data.best_hand.net_change.toLocaleString('pt-BR')} fichas</b>
                    <span>{new Date(handEndedAtMs(showcase.data.best_hand.ended_at)).toLocaleDateString('pt-BR')}</span>
                  </article> : <p className="profile-showcase-empty">Nenhuma vitória recente registrada nesta vitrine.</p>}
                </section>;
                return matchup.data && matchup.data.hands_together > 0
                  ? <MatchupCard key={id} stats={matchup.data} opponentName={showcase.data.name || 'Jogador'}/>
                  : null;
              })}
            </div>;
          })()}
        </>}
    </ShowcaseShell>
  </AppPage>;
}

function MatchupCard({stats, opponentName}: {stats: MatchupStats; opponentName: string}) {
  const {hands_together, viewer_wins, opponent_wins, ties, heads_up_hands_together, net_change_viewer} = stats;
  const net = net_change_viewer;
  const netClass = net > 0 ? 'gain' : net < 0 ? 'loss' : 'even';
  return <section className="profile-matchup">
    <h2><Swords aria-hidden="true"/> Cara a Cara</h2>
    <p>
      Vocês já jogaram {hands_together.toLocaleString('pt-BR')} mãos juntos: você venceu
      {' '}{viewer_wins.toLocaleString('pt-BR')}, {opponentName} venceu {opponent_wins.toLocaleString('pt-BR')}
      {ties > 0 ? `, e ${ties.toLocaleString('pt-BR')} terminaram empatadas.` : '.'}
    </p>
    <p className="profile-matchup-net">
      Saldo de fichas nesse confronto:{' '}
      <strong className={`hand-net ${netClass}`}>
        {net > 0 ? '+' : ''}{net.toLocaleString('pt-BR')} fichas
      </strong>
      {net > 0 ? ' a seu favor.' : net < 0 ? ' contra você.' : ' — empatado.'}
    </p>
    {heads_up_hands_together > 0 && heads_up_hands_together !== hands_together && (
      <p className="profile-matchup-headsup">
        {heads_up_hands_together.toLocaleString('pt-BR')} dessas mãos foram mano a mano, só vocês dois na mesa.
      </p>
    )}
  </section>;
}

export default function ProfilePage() {
  return <Suspense fallback={<main className="app-page">
    <section className="profile-showcase shell">
      <h1 className="sr-only">Perfil do jogador</h1>
      <LoadingRegion label="Carregando perfil…" className="skeleton-panel profile-showcase-skeleton">
        <Skeleton style={{height: '68px', width: '68px', borderRadius: '50%'}}/>
        <Skeleton style={{height: '26px', width: 'min(260px, 70%)'}}/>
        <Skeleton style={{height: '150px'}}/>
      </LoadingRegion>
    </section>
  </main>}>
    <ProfileContent/>
  </Suspense>;
}
