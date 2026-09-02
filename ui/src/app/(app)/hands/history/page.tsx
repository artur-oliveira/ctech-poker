'use client';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import {Suspense, useState} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Coins,
  Copy,
  Crown,
  Handshake,
  ListChecks,
  Play,
  ShieldCheck,
  Table2
} from 'lucide-react';
import type {WalletMode} from '@/lib/api/player';
import {getHand} from '@/lib/api/player';
import {getHandHistory} from '@/lib/api/table';
import {getPlayerNotes, type PlayerNote} from '@/lib/api/playerNotes';
import {getRelationships} from '@/lib/api/social';
import {PlayingCard} from '@/components/table/PlayingCard';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {BoardSlots} from '@/components/hands/BoardSlots';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {ActionTimeline} from '@/components/hands/ActionTimeline';
import {LoadingRegion, Skeleton, SkeletonList} from '@/components/ui/skeleton';
import {DeckReveal} from '@/components/hands/DeckReveal';
import {PartialDeckProof} from '@/components/hands/PartialDeckProof';
import {HandExportButton} from '@/components/hands/HandExportButton';
import {ShareHandDialog} from '@/components/hands/ShareHandDialog';
import {PlayerActionsMenu} from '@/components/social/PlayerActionsMenu';
import {Button} from '@/components/ui/button';
import {TermsGate} from '@/components/TermsGate';
import {RecoveryState} from '@/components/RecoveryState';
import {getViewerId, HAND_CATEGORY_LABELS, playerName} from '@/lib/utils';
import {bestHandCategory} from '@/lib/pokerRules';
import {availableWalletMode} from '@/lib/capabilities';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {SOCIAL_KEYS} from '@/lib/social';

// Loaded lazily: the private-note editor is only ever opened from the
// opponent-menu affordance below, so most visits to a hand's detail never
// need its bundle.
const PlayerNoteDialog = dynamic(() => import('@/components/table/PlayerNoteDialog')
  .then(module => module.PlayerNoteDialog));

function formatDate(unixSeconds: number) {
  return new Date(unixSeconds * 1000).toLocaleString('pt-BR', {
    day: '2-digit', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit'
  });
}

function HandHistoryContent() {
  const params = useSearchParams();
  const tableId = params.get('table_id') || '';
  const handId = params.get('hand_id') || '';
  const mode: WalletMode = availableWalletMode(params.get('mode'));
  const [tableCopied, setTableCopied] = useState(false);
  const [noteOpponent, setNoteOpponent] = useState<{ player_id: string; name?: string } | null>(null);
  const socialActions = useSocialActions();
  const queryClient = useQueryClient();

  const hand = useQuery({
    queryKey: ['hand', mode, handId],
    queryFn: () => getHand(handId, mode),
    enabled: Boolean(handId)
  });
  const history = useQuery({
    queryKey: ['hand-history', tableId, handId],
    queryFn: () => getHandHistory(tableId, handId),
    enabled: Boolean(tableId && handId)
  });

  const actions = [...(history.data?.actions || [])].sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
  const viewerId = getViewerId();
  const opponentNames = new Map((hand.data?.opponents || []).map(o => [o.player_id, o.name]));
  const resolveName = (playerId: string) => playerName(playerId, viewerId, opponentNames.get(playerId));

  // Menu affordances (profile actions + private note) require a signed-in
  // viewer; a logged-out reader of a shared/incomplete-session link never
  // sees them. `getViewerId()` is the same "who is looking at this" gate
  // used everywhere else in the app.
  const opponentIds = Boolean(viewerId) ? (hand.data?.opponents || [])
    .map(o => o.player_id).filter((id): id is string => Boolean(id)) : [];
  const {data: relationships = []} = useQuery({
    queryKey: SOCIAL_KEYS.relationships(opponentIds), queryFn: () => getRelationships(opponentIds),
    enabled: opponentIds.length > 0
  });
  const relationshipsByID = Object.fromEntries(relationships.map(item => [item.player_id, item]));
  const {data: playerNotes = []} = useQuery({
    queryKey: ['player-notes'], queryFn: getPlayerNotes, enabled: opponentIds.length > 0
  });
  const playerNotesByID = Object.fromEntries(playerNotes.map(note => [note.opponent_id, note]));

  // A link that lost its parameters is a trust moment, not a stray error line:
  // it gets the same recovery composition the replay page uses.
  if (!tableId || !handId) return <RecoveryState
    nested
    title="Este link de mão está incompleto"
    description="O endereço não diz qual mesa e qual mão abrir. Suas mãos continuam registradas — escolha uma na lista para ver o detalhe completo."
    action={<Button render={<Link href="/hands"/>}><ListChecks/> Ver minhas mãos</Button>}/>;

  // Shaped like the loaded page (tool row, result header, seats, board, timeline)
  // so nothing jumps when the hand arrives.
  if (hand.isLoading) return <div className="hand-history shell static-cards">
    <LoadingRegion label="Carregando detalhes da mão…" className="skeleton-panel hand-history-skeleton">
      <Skeleton style={{height: '20px', width: '190px'}}/>
      <Skeleton style={{height: '104px'}}/>
      <Skeleton style={{height: '170px'}}/>
      <Skeleton style={{height: '140px'}}/>
    </LoadingRegion>
  </div>;

  if (hand.isError || !hand.data) return <RecoveryState
    nested
    title="Não foi possível carregar esta mão"
    description="A mão pode não pertencer a esta conta ou o histórico não está mais disponível. Verifique se você entrou com a conta certa."
    action={<Button render={<Link href="/hands"/>}><ListChecks/> Ver minhas mãos</Button>}/>;

  const h = hand.data;
  const viewerIsWinner = h.outcome === 'won' || h.outcome === 'tied';

  function categoryFor(holeCards?: string[]): string | null {
    if (holeCards?.length !== 2 || h.board?.length !== 5) return null;
    return HAND_CATEGORY_LABELS[bestHandCategory([...holeCards, ...h.board])] || null;
  }

  const viewerCategory = categoryFor(h.hole_cards);

  async function copyTableId() {
    try {
      await navigator.clipboard.writeText(h.table_id);
      setTableCopied(true);
    } catch {
      setTableCopied(false);
    }
  }

  return <div className="hand-history shell static-cards">
    {/* Export and share are utilities: they belong on the page's tool row beside
        the way out, not stacked down the centre line where the result and the
        winners are what the player came to read. */}
    <div className="hand-history-topbar">
      <Link href="/hands"><ChevronLeft/> Voltar para Minhas Mãos</Link>
      <div className="hand-history-tools">
        <HandExportButton hand={h} actions={actions} viewerId={viewerId}
                          actionsAvailable={!history.isLoading && !history.isError}/>
        <ShareHandDialog handId={h.hand_id} outcome={h.outcome} mode={mode}/>
      </div>
    </div>
    <header className={`hand-history-header is-${h.outcome}`}>
      <OutcomeBadge outcome={h.outcome}/>
      <h1>Detalhes da mão</h1>
      <div className="hand-history-meta">
        <span><CalendarDays aria-hidden="true"/>{formatDate(h.ended_at / 1000)}</span>
        <span className="hand-history-table-id"><Table2 aria-hidden="true"/>Mesa <code>{h.table_id}</code>
          <button type="button" onClick={() => void copyTableId()}
                  aria-label={tableCopied ? 'ID da mesa copiado' : 'Copiar ID da mesa'} title="Copiar ID da mesa">
            <Copy aria-hidden="true"/>{tableCopied && <span>Copiado</span>}
          </button>
        </span>
        <span><Coins aria-hidden="true"/>{mode === 'real' ? 'Dinheiro real' : 'Sandbox'}</span>
      </div>
      <div className="hand-history-net">
        <small>Resultado líquido</small>
        <strong className={`hand-net large ${h.net_change > 0 ? 'gain' : h.net_change < 0 ? 'loss' : 'even'}`}>
          {h.net_change > 0 ? '+' : ''}{h.net_change.toLocaleString('pt-BR')} fichas
        </strong>
      </div>
    </header>

    {!history.isLoading && !history.isError && actions.some(action => action.frame) &&
        <div className="hand-replay-launch">
            <div>
                <Play aria-hidden="true"/>
                <span>
            <b>Reviva esta mão ação por ação</b>
            <small>O replayer abre nesta mesma aba; o botão de voltar traz você de volta para cá.</small>
          </span>
            </div>
          {/* Same tab on purpose: the replayer's own "Voltar para Detalhes da
                Mão" is a back link, and a new tab left the player with two
                windows on the same hand and no way back in either. */}
            <Button render={<Link
              href={`/hands/replay?table_id=${encodeURIComponent(tableId)}&hand_id=${encodeURIComponent(handId)}&mode=${mode}`}/>}>
                Assistir replay <ChevronRight aria-hidden="true"/>
            </Button>
        </div>}

    <section className="hand-history-players">
      <article className={`hand-history-seat viewer${viewerIsWinner ? ' is-winner' : ''}`}>
        {h.outcome === 'tied'
          ? <Handshake aria-hidden="true" className="winner-crown tie-mark"/>
          : viewerIsWinner && <Crown aria-hidden="true" className="winner-crown"/>}
        <b>Você</b>
        <div className="hand-history-seat-cards">
          {(h.hole_cards || []).map((c, i) => <PlayingCard key={i} card={c} index={i} size="hole" owner="viewer"/>)}
        </div>
        {viewerCategory && <small className="hand-category">{viewerCategory}</small>}
      </article>
      {(h.opponents || []).map((o, i) => {
        const category = categoryFor(o.hole_cards);
        const identity = <>
          <PlayerAvatar name={o.name} avatarUrl={o.avatar_url} size={36}/>
          <b>{o.name || 'Adversário'}</b>
        </>;
        const relationship = o.player_id ? relationshipsByID[o.player_id] : undefined;
        return <article key={o.player_id || `opponent-${i}`}
                        className={`hand-history-seat${o.won ? ' is-winner' : ''}`}>
          {o.won && (h.outcome === 'tied'
            ? <Handshake aria-hidden="true" className="winner-crown tie-mark"/>
            : <Crown aria-hidden="true" className="winner-crown"/>)}
          {/* A degenerate row with no player_id (legacy data) keeps the plain
              text+avatar it always had — no profile to link to and no player
              to run a social action against. */}
          {o.player_id
            ? <Link href={`/profile?id=${encodeURIComponent(o.player_id)}`} className="hand-history-seat-identity">
              {identity}
            </Link>
            : identity}
          {o.player_id && viewerId && <div className="hand-history-seat-actions">
              <PlayerActionsMenu
                target={{
                  player_id: o.player_id, name: o.name,
                  relationship: relationship?.relationship,
                  muted: relationship?.muted, blocked: relationship?.blocked
                }}
                actions={socialActions} surface="table_behavior" tableId={h.table_id} handId={h.hand_id}
                onEditNoteAction={() => setNoteOpponent({player_id: o.player_id, name: o.name})}/>
          </div>}
          <div className="hand-history-seat-cards">
            {o.hole_cards?.length
              ? o.hole_cards.map((c, i) => <PlayingCard key={i} card={c} index={i} size="hole"/>)
              : <><span className="board-slot-undealt" aria-hidden="true">
                  <PlayingCard index={0} size="hole"/><PlayingCard index={1} size="hole"/>
                </span>
                <span className="hand-history-seat-hidden">Cartas não reveladas</span></>}
          </div>
          {category && <small className="hand-category">{category}</small>}
        </article>;
      })}
    </section>

    <section className="hand-history-board" aria-label="Board final da mão">
      <h2><Table2 aria-hidden="true"/> Cartas comunitárias</h2>
      <div className="hand-board" aria-label="Cartas comunitárias">
        <BoardSlots board={h.board}/>
      </div>
    </section>

    <section className="hand-history-actions">
      <h2><ListChecks aria-hidden="true"/> Histórico de ações</h2>
      {history.isLoading ?
        <SkeletonList label="Carregando histórico de ações…" count={5} height={44} className="skeleton-panel"/> :
        history.isError ? <div className="hand-history-partial-error" role="alert">
          <p className="form-error">Não foi possível carregar a sequência de ações. O resumo, a prova e as ferramentas continuam disponíveis.</p>
          <Button variant="outline" size="sm" onClick={() => void history.refetch()}>Tentar ações novamente</Button>
        </div> :
          <ActionTimeline actions={actions} resolveName={resolveName}/>}
    </section>

    <section className="hand-history-fairness">
      <h2><ShieldCheck aria-hidden="true"/> Prova de integridade</h2>
      {h.server_seed && h.commit_hash
        ? <DeckReveal key={h.hand_id} serverSeed={h.server_seed} commitHash={h.commit_hash}/>
        : h.root_commit_hash && h.revealed_card_salts && h.unrevealed_card_hashes
          ? <PartialDeckProof key={h.hand_id} rootCommitHash={h.root_commit_hash}
                              revealed={h.revealed_card_salts} unrevealed={h.unrevealed_card_hashes}/>
          :
          <p className="deck-reveal-status mismatch">Prova de integridade criptográfica indisponível para esta mão.</p>}
    </section>

    <PlayerNoteDialog key={noteOpponent?.player_id || 'closed'} opponent={noteOpponent}
                      existing={noteOpponent ? playerNotesByID[noteOpponent.player_id] : undefined}
                      open={Boolean(noteOpponent)}
                      onOpenChangeAction={open => !open && setNoteOpponent(null)}
                      onSaved={(note: PlayerNote | null) => {
                        if (!noteOpponent) return;
                        queryClient.setQueryData<PlayerNote[]>(['player-notes'], current => {
                          const rest = (current || []).filter(item => item.opponent_id !== noteOpponent.player_id);
                          return note ? [...rest, note] : rest;
                        });
                      }}/>
  </div>;
}

export default function HandHistoryPage() {
  return <TermsGate>
    <main className="app-page">
      <Suspense fallback={<div className="hand-history shell static-cards">
        <LoadingRegion label="Carregando mão…" className="skeleton-panel hand-history-skeleton">
          <Skeleton style={{height: '20px', width: '190px'}}/>
          <Skeleton style={{height: '104px'}}/>
          <Skeleton style={{height: '170px'}}/>
        </LoadingRegion>
      </div>}>
        <HandHistoryContent/>
      </Suspense>
    </main>
  </TermsGate>;
}
