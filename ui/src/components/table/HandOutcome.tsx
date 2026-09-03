'use client';
import {useEffect, useState} from 'react';
import {Equal, Flag, Layers3, PartyPopper, Repeat2, Swords, X} from 'lucide-react';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {bestHandCategory, HAND_MATCH_SIZE, wasDecidedByKicker} from '@/lib/pokerRules';
import {PlayingCard} from '@/components/table/PlayingCard';
import {ChipStack} from '@/components/table/ChipStack';
import {PerimeterTimer} from '@/components/table/PerimeterTimer';
import {useCountUp} from '@/lib/hooks/useCountUp';

export type HandOutcomeState = {
  key: number; kind: 'win' | 'lose' | 'tie' | 'mixed' | 'fold'; handCategory?: string; opponentCategory?: string;
  // Only set when kind is 'fold': whether the viewer's own hole cards would
  // actually have beaten the eventual winner's revealed hand had they stayed
  // in. Undefined when the hand never reached a showdown (no one's cards to
  // compare against), which reads as the plain "you folded" message instead.
  couldHaveWon?: boolean;
  // The winning 5-card hand (or just the 2 hole cards when the board isn't
  // complete) for whoever actually won this pot: the viewer's own on a win,
  // the rival's on a loss. Undefined when the hand ended without a
  // showdown, since no one's cards were ever revealed to compare.
  winningCards?: string[];
  // The 2 hole cards belonging to that same winning hand, retained so a win
  // can identify which combination cards came from the viewer.
  winningHoleCards?: string[];
  // The viewer's own resolved hand is also retained on a loss so the result
  // can explain the showdown as an actual hand-to-hand comparison.
  viewerCards?: string[];
  viewerHoleCards?: string[];
  winnerName?: string;
  // The hand the viewer beat to take this pot, and its category — only set
  // on a win, and only when that opponent's cards were actually revealed at
  // showdown. Lets a win explain "same combination, the kicker decided" too,
  // not just a loss.
  beatenCards?: string[];
  beatenCategory?: string;
  // The viewer's stack right before this hand resolved and right after. The
  // chip counter below animates between the two, up when they gained chips,
  // down when they lost some, and stays hidden when neither changed (e.g. a
  // free showdown they simply lost with nothing left in the pot for them).
  stackBefore?: number;
  stackAfter?: number;
  wonAmount?: number;
  refundAmount?: number;
  // Only set when kind is 'mixed': every pot the viewer contested, in the
  // order the server resolved them. A won entry carries no extra data since
  // the viewer's own hand (above, `viewerCards`) is the same hand for every
  // pot; a lost entry names the actual winner of that specific pot, which
  // can differ pot to pot (e.g. two different rivals each win a side pot the
  // viewer wasn't eligible for the other's).
  pots?: ({ won: true } | { won: false; winnerName?: string; category?: string; winningCards?: string[] })[];
  // Only set when kind is 'tie': the other player(s) who split this pot with
  // the viewer, so a 2-way or 3+-way chop can show every tied hand instead
  // of only the viewer's own combination. `cards` is undefined whenever that
  // player's hole cards weren't revealed.
  tiedWith?: { name?: string; cards?: string[] }[];
  runItTwice?: boolean;
  // Complete, server-authored settlement of every pot layer. This is wider
  // than `pots` above: a viewer may win the main pot while being ineligible
  // for a side pot, and still needs to understand where the rest went.
  resolvedPots?: {
    amount: number;
    payoutAmount: number;
    viewerPayout?: number;
    winnerNames: string[];
    wonByViewer: boolean;
    viewerEligible: boolean;
    split: boolean;
    refund: boolean;
    runout?: number;
  }[];
};

const EXIT_MS = 320;
const CONFETTI_PIECES = Array.from({length: 8}, (_, i) => i);
const CHIP_COUNT_MS = 700;
// Icon/label for the collapsed badge, one per outcome kind, so a dismissed
// win still reads as a win at a glance instead of a bare "something happened"
// dot. Mirrors the icons the full card already uses for win/tie/mixed;
// lose and fold have none on the full card (their heading is text-only), so
// these are new but chosen to fit: crossed hands lost the showdown, a raised
// flag reads as "folded" without needing new copy.
const BADGE_ICON = {win: PartyPopper, lose: Swords, tie: Equal, mixed: Equal, fold: Flag} as const;
const BADGE_LABEL: Record<HandOutcomeState['kind'], string> = {
  win: 'Ver resultado: você venceu', lose: 'Ver resultado: você perdeu',
  tie: 'Ver resultado: pote dividido', mixed: 'Ver resultado misto', fold: 'Ver resultado: você desistiu'
};

function categoryFor(cards?: string[], fallback?: string): string | undefined {
  return cards?.length === 5 ? bestHandCategory(cards) : fallback;
}

function categoryLabel(category?: string): string | undefined {
  return category && (HAND_CATEGORY_LABELS[category] || category);
}

// The resolved five cards include kickers because poker needs them to rank a
// hand. The outcome does not: it shows only the cards that visibly make the
// named combination (two for a pair, four for two pair, all five for a
// straight, and so on). A hand won without showdown has no revealed
// combination, so it deliberately renders no cards.
function combinationCards(cards?: string[], fallbackCategory?: string): string[] {
  if (cards?.length !== 5) return [];
  const category = categoryFor(cards, fallbackCategory);
  return cards.slice(0, category ? HAND_MATCH_SIZE[category] ?? cards.length : cards.length);
}

// Same combination as combinationCards, but with the kicker(s) appended
// after it, for a showdown lost or won by kicker within the same hand
// category, where naming just "Par" for both sides hides the actual reason
// one beat the other.
function combinationWithKickers(cards?: string[], fallbackCategory?: string): { cards: string[]; kickerFrom: number } {
  const combination = combinationCards(cards, fallbackCategory);
  if (!combination.length || cards?.length !== 5) return {cards: combination, kickerFrom: combination.length};
  return {cards, kickerFrom: combination.length};
}

function OutcomeCards({cards, viewerHoleCards, startIndex = 0, kickerFrom}: {
  cards: string[];
  viewerHoleCards?: string[];
  startIndex?: number;
  kickerFrom?: number;
}) {
  const viewerCards = new Set(viewerHoleCards);
  if (!cards.length) return null;
  return <span className="hand-outcome-cards">
    {cards.map((card, index) => {
      const isViewerCard = viewerCards.has(card);
      const isKicker = kickerFrom != null && index >= kickerFrom;
      return <span key={card}
                   className={`hand-outcome-card-slot${isViewerCard ? ' is-viewer' : ''}${isKicker ? ' is-kicker' : ''}`}>
        <PlayingCard card={card} index={startIndex + index} size="hole" owner={isViewerCard ? 'viewer' : undefined}/>
      </span>;
    })}
  </span>;
}

function OutcomeSettlement({pots = [], runItTwice = false}: {
  pots?: HandOutcomeState['resolvedPots'];
  runItTwice?: boolean;
}) {
  if (!runItTwice && pots.length < 2) return null;
  return <section className="hand-outcome-settlement" aria-label="Distribuição dos potes">
    <header>
      {runItTwice ? <Repeat2 aria-hidden="true"/> : <Layers3 aria-hidden="true"/>}
      <span><b>{runItTwice ? 'Rodado duas vezes' : 'Acerto dos potes'}</b>
        <small>{runItTwice ? 'Dois boards, resultados confirmados' : `${pots.length} potes resolvidos`}</small></span>
    </header>
    {pots.length > 0 && <ul>
      {pots.map((pot, index) => {
        const label = index === 0 ? 'Pote principal' : `Pote lateral ${index}`;
        const winner = pot.refund ? 'Devolução' : pot.wonByViewer ? 'Você' :
          pot.winnerNames.length ? pot.winnerNames.join(' e ') : 'Sem vencedor';
        const payout = pot.split ? `${winner} · ${pot.payoutAmount.toLocaleString('pt-BR')} fichas distribuídas${
          pot.viewerPayout != null ? ` · sua parte +${pot.viewerPayout.toLocaleString('pt-BR')}` : ''}` :
          `${winner}${pot.payoutAmount > 0 ? ` +${pot.payoutAmount.toLocaleString('pt-BR')}` : ''}`;
        return <li key={`${index}-${pot.amount}-${pot.runout ?? 0}`}
                   className={pot.wonByViewer ? 'viewer-won' : pot.viewerEligible ? 'viewer-lost' : 'viewer-out'}>
          <span><b>{label}{pot.runout ? ` · Board ${pot.runout}` : ''}</b>
            {!pot.viewerEligible && !pot.refund && <small>Você não disputou este pote</small>}</span>
          <span className="hand-outcome-pot-amount">{pot.amount.toLocaleString('pt-BR')}</span>
          <strong>{pot.split ? 'Dividido · ' : ''}{payout}</strong>
        </li>;
      })}
    </ul>}
  </section>;
}

/** Wraps PerimeterTimer with the same "capture elapsed time once, at mount"
 * pattern Seat.tsx's SeatTurnTimer uses. Needed here because the ring's host
 * (dismiss button / badge / standalone dot) swaps in and out as the card is
 * dismissed and reopened — each swap is a fresh DOM mount, and without a
 * captured elapsed offset the CSS animation would restart from full every
 * time, instead of continuing the same countdown. Callers must key this by
 * `deadlineMs` so a genuinely new deadline (not just a dismiss toggle) also
 * re-captures a fresh elapsed offset.
 *
 * Reads Date.now() directly rather than taking a `nowMs` prop: the seat
 * timers this pattern is copied from only remount when the deadline itself
 * changes, so their caller's `nowMs` (the last snapshot's arrival time) is
 * still fresh at that instant. This ring also remounts on every dismiss/
 * reopen toggle, a moment with no snapshot of its own — a snapshot-cadence
 * `nowMs` would be stale by however long the player waited before clicking,
 * which is what made every dismiss recapture elapsed as if just-armed. */
function HandOutcomeRing({className, radius, deadlineMs, durationMs}: {
  className: string;
  radius: number;
  deadlineMs: number;
  durationMs: number;
}) {
  const [elapsedMs] = useState(() => Math.max(0, Date.now() - (deadlineMs - durationMs)));
  return <PerimeterTimer className={className} durationMs={durationMs} elapsedMs={elapsedMs}
                         restartKey={deadlineMs} radius={radius}/>;
}

/** Three-beat reveal of a stack change: the stack as it was, the delta that's
 * about to land, then the two merging into one counted total. Counts up
 * on a gain, down on a loss, since the same sequence reads honestly either
 * way. Skips straight to the merged total under reduced motion instead of
 * dropping the animation silently, since the number itself still has to end up
 * correct. */
function ChipCountUp({from, to}: { from: number; to: number }) {
  const delta = to - from;
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const [phase, setPhase] = useState<'base' | 'delta' | 'counting'>(reduced ? 'counting' : 'base');
  
  // No dependency on from/to to reset `phase`: this component lives under a
  // parent keyed by the outcome's hand key, so a new hand remounts it fresh
  // (phase starts over at its initial value) instead of needing a manual
  // reset here.
  useEffect(() => {
    if (reduced) return () => {
    };
    const toDelta = setTimeout(() => setPhase('delta'), 260);
    const toCounting = setTimeout(() => setPhase('counting'), 560);
    return () => {
      clearTimeout(toDelta);
      clearTimeout(toCounting);
    };
  }, [reduced]);
  
  const display = useCountUp(from, phase === 'counting' ? to : from, CHIP_COUNT_MS);
  const sign = delta > 0 ? '+' : '−';
  return <span className={`hand-outcome-chips ${delta > 0 ? 'gain' : 'loss'}`}>
    {phase === 'base' && <span key="base" className="hand-outcome-chips-base">
      {from.toLocaleString('pt-BR')} fichas</span>}
    {phase === 'delta' && <span key="delta" className="hand-outcome-chips-delta">
      <span>{from.toLocaleString('pt-BR')}</span><b>{sign}{Math.abs(delta).toLocaleString('pt-BR')}</b></span>}
    {phase === 'counting' && <span key="counting" className="hand-outcome-chips-total">
      {delta > 0 && <ChipStack amount={delta} size="pot"/>}{display.toLocaleString('pt-BR')} fichas</span>}
  </span>;
}

/** Fires once per resolved hand (keyed by an ever-increasing counter the
 * caller bumps only when payouts first appear for that hand), never on the
 * repeat broadcasts that follow while the table sits in `complete`. Purely
 * decorative: the sr-only live region already announces the same outcome.
 *
 * Stays open for as long as the table itself is still showing that resolved
 * hand's payouts (`holdOpen`) instead of a fixed timer, so a player who glances
 * away mid-hand and looks back a few seconds later still finds their
 * win/loss on screen, not a banner that already auto-dismissed under them.
 * It closes once the next hand actually starts. */
export function HandOutcomeBanner({outcome, holdOpen, nextHandDeadlineMs, nextHandDurationMs,
                                   onDismissedChangeAction}: {
  outcome: HandOutcomeState | null;
  holdOpen: boolean;
  onDismissedChangeAction?: (dismissed: boolean) => void;
  // Countdown to the next hand starting. Rendered as a ring on the dismiss
  // button (full card) or around the badge (collapsed), instead of its old
  // home floating over the felt's center (.street-progress): a winner's
  // payout chips float up from their seat toward that same center point,
  // and the two collided there on desktop. This corner is always clear.
  nextHandDeadlineMs?: number;
  nextHandDurationMs?: number;
}) {
  const [shown, setShown] = useState(outcome);
  // Collapsed to the corner badge until the player dismisses the full card,
  // or reopens it. Reset per hand (not per broadcast) so a dismissal never
  // carries over and silently hides the next hand's result.
  const [dismissedKey, setDismissedKey] = useState<number | null>(null);
  const dismissed = dismissedKey !== null && dismissedKey === outcome?.key;

  useEffect(() => {
    if (outcome) {
      // Retain the last outcome while its exit animation finishes.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setShown(previous => previous?.key === outcome.key ? previous : outcome);
    }
  }, [outcome]);

  // Only the parent notification is a side effect; the flag itself is derived
  // from which hand was dismissed, so a new hand_id un-dismisses it for free.
  useEffect(() => {
    onDismissedChangeAction?.(false);
  }, [outcome?.key, onDismissedChangeAction]);

  const leaving = !!shown && !holdOpen;
  
  useEffect(() => {
    if (!leaving) return () => {
    };
    const clear = setTimeout(() => setShown(null), EXIT_MS);
    return () => clearTimeout(clear);
  }, [leaving]);
  
  if (!shown) {
    // No personalized outcome to show yet (e.g. a reload landing mid
    // `complete` stage before the client can locally diff the payout that
    // produces one) but the next hand is still armed: keep the countdown
    // visible in the same corner the badge would use, rather than dropping
    // it entirely.
    if (nextHandDeadlineMs == null) return null;
    return <div className="hand-outcome">
      <span className="hand-outcome-ring-standalone" aria-hidden="true">
        <HandOutcomeRing key={nextHandDeadlineMs} className="hand-outcome-ring" radius={7}
                         deadlineMs={nextHandDeadlineMs} durationMs={nextHandDurationMs ?? 0}/>
      </span>
    </div>;
  }
  const ownCategory = categoryFor(shown.viewerCards || shown.winningCards, shown.handCategory);
  const winnerCategory = categoryFor(shown.winningCards, shown.opponentCategory);
  const ownCombination = combinationCards(shown.viewerCards || shown.winningCards, shown.handCategory);
  const winningCombination = combinationCards(shown.winningCards, shown.opponentCategory);
  // Naming the same category for both sides ("Par" vs. "Par") hides why one
  // beat the other, so show the kicker(s) that actually broke the tie instead.
  const sameCategory = shown.kind === 'lose' && ownCategory && ownCategory === winnerCategory;
  const decidedByKicker = Boolean(sameCategory && shown.viewerCards && shown.winningCards &&
    wasDecidedByKicker(shown.viewerCards, shown.winningCards));
  const higherCombination = Boolean(sameCategory && !decidedByKicker);
  // Mirror of the above for a win: did the viewer's own kicker beat an
  // opponent who made the very same combination?
  const beatenCategory = categoryFor(shown.beatenCards, shown.beatenCategory);
  const sameCategoryWin = shown.kind === 'win' && ownCategory && ownCategory === beatenCategory;
  const wonByKicker = Boolean(sameCategoryWin && shown.viewerCards && shown.beatenCards &&
    wasDecidedByKicker(shown.viewerCards, shown.beatenCards));
  const ownWithKickers = combinationWithKickers(shown.viewerCards, shown.handCategory);
  const winningWithKickers = combinationWithKickers(shown.winningCards, shown.opponentCategory);
  const chipChange = shown.stackBefore != null && shown.stackAfter != null &&
  shown.stackBefore !== shown.stackAfter
    ? <ChipCountUp from={shown.stackBefore} to={shown.stackAfter}/>
    : null;
  const amountDetails = [
    shown.wonAmount ? `${shown.wonAmount.toLocaleString('pt-BR')} fichas ganhas` : '',
    shown.refundAmount ? `${shown.refundAmount.toLocaleString('pt-BR')} fichas devolvidas` : ''
  ].filter(Boolean).join(' e ');
  const announcement = shown.kind === 'win'
    ? `Você venceu com ${categoryLabel(ownCategory) || 'a mão mais forte'}` +
    `${wonByKicker ? ', decidido pelo kicker' : ''}${amountDetails ? `: ${amountDetails}` : ''}.`
    : shown.kind === 'lose'
      ? `Você perdeu esta mão${shown.winnerName ? `. Vencedor: ${shown.winnerName}` : ''}` +
      `${categoryLabel(winnerCategory) ? ` com ${categoryLabel(winnerCategory)}` : ''}` +
      `${decidedByKicker ? ', decidido pelo kicker' : higherCombination ? ', combinação mais alta venceu' : ''}` +
      `${amountDetails ? `. ${amountDetails}` : ''}.`
      : shown.kind === 'tie'
        ? `Pote dividido${categoryLabel(ownCategory) ? ` com ${categoryLabel(ownCategory)}` : ''}` +
        `${amountDetails ? `: ${amountDetails}` : ''}.`
        : shown.kind === 'fold'
          ? shown.couldHaveWon ? 'Você desistiu, mas sua mão venceria a mão revelada.' : 'Você desistiu desta mão.'
          : `Resultado misto: você ganhou ao menos um pote e perdeu outro${amountDetails ? `. ${amountDetails}` : ''}.`;
  const BadgeIcon = BADGE_ICON[shown.kind];

  return <>
    <span className="sr-only" role="status" aria-live="polite">{announcement}</span>
    <div className="hand-outcome">
      {dismissed ? (
        <button type="button" className={`hand-outcome-badge ${shown.kind}${leaving ? ' leaving' : ''}`}
                onClick={() => { setDismissedKey(null); onDismissedChangeAction?.(false); }}
                aria-label={BADGE_LABEL[shown.kind]}>
          {nextHandDeadlineMs != null && <HandOutcomeRing key={nextHandDeadlineMs} className="hand-outcome-ring"
            radius={20} deadlineMs={nextHandDeadlineMs} durationMs={nextHandDurationMs ?? 0}/>}
          <BadgeIcon aria-hidden="true"/>
        </button>
      ) : (
      <div key={shown.key} className={`hand-outcome-card ${shown.kind}${leaving ? ' leaving' : ''}`}>
        <button type="button" className="hand-outcome-dismiss"
                onClick={() => { setDismissedKey(outcome?.key ?? null); onDismissedChangeAction?.(true); }}
                aria-label="Minimizar resultado">
          {nextHandDeadlineMs != null && <HandOutcomeRing key={nextHandDeadlineMs} className="hand-outcome-ring"
            radius={14} deadlineMs={nextHandDeadlineMs} durationMs={nextHandDurationMs ?? 0}/>}
          <X aria-hidden="true"/>
        </button>
        {shown.kind === 'win' && <>
            <span className="hand-outcome-confetti" aria-hidden="true">{CONFETTI_PIECES.map(i => <span key={i}/>)}</span>
            <div className="hand-outcome-heading">
                <PartyPopper aria-hidden="true"/>
                <span><b>Você venceu!</b><small>{categoryLabel(ownCategory) || 'Pote conquistado'}</small></span>
            </div>
            <OutcomeCards cards={wonByKicker ? ownWithKickers.cards : ownCombination}
                          kickerFrom={wonByKicker ? ownWithKickers.kickerFrom : undefined}
                          viewerHoleCards={shown.viewerHoleCards || shown.winningHoleCards}/>
          {wonByKicker && <p className="hand-outcome-kicker-note">Mesma combinação, o kicker decidiu.</p>}
          {amountDetails && <p className="hand-outcome-tie-note">{amountDetails}</p>}
        </>}

        {shown.kind === 'lose' && <>
            <div className="hand-outcome-heading">
                <span><b>Não foi dessa vez.</b><small>Veja o confronto final</small></span>
            </div>
            <div className="hand-outcome-comparison">
                <div className="hand-outcome-comparison-row winner">
            <span className="hand-outcome-hand-name">
              <small>{shown.winnerName || 'Vencedor'}</small>
              <strong>{categoryLabel(winnerCategory) || 'Mão vencedora'}</strong>
            </span>
                    <OutcomeCards cards={decidedByKicker ? winningWithKickers.cards : winningCombination}
                                  kickerFrom={decidedByKicker ? winningWithKickers.kickerFrom : undefined}
                                  startIndex={0}/>
                </div>
                <span className="hand-outcome-versus">venceu</span>
                <div className="hand-outcome-comparison-row viewer">
            <span className="hand-outcome-hand-name">
              <small>Você</small>
              <strong>{categoryLabel(ownCategory) || 'Sua mão'}</strong>
            </span>
                    <OutcomeCards cards={decidedByKicker ? ownWithKickers.cards : ownCombination}
                                  kickerFrom={decidedByKicker ? ownWithKickers.kickerFrom : undefined}
                                  viewerHoleCards={shown.viewerHoleCards} startIndex={5}/>
                </div>
            </div>
          {decidedByKicker && <p className="hand-outcome-kicker-note">Mesma combinação, o kicker decidiu.</p>}
          {higherCombination && <p className="hand-outcome-kicker-note">A combinação mais alta venceu.</p>}
          {amountDetails && <p className="hand-outcome-tie-note">{amountDetails}</p>}
        </>}
        
        {shown.kind === 'tie' && <>
            <div className="hand-outcome-heading">
                <Equal aria-hidden="true"/>
                <span><b>Pote dividido</b><small>{categoryLabel(ownCategory) || 'Combinação empatada'}</small></span>
            </div>
          {shown.tiedWith?.length ? <div className="hand-outcome-comparison">
              <div className="hand-outcome-comparison-row viewer">
            <span className="hand-outcome-hand-name">
              <small>Você</small>
              <strong>{categoryLabel(ownCategory) || 'Sua mão'}</strong>
            </span>
                  <OutcomeCards cards={ownCombination} viewerHoleCards={shown.viewerHoleCards || shown.winningHoleCards}
                                startIndex={0}/>
              </div>
            {shown.tiedWith.map((tied, index) => <div key={index} className="hand-outcome-comparison-row tied">
              <span className="hand-outcome-hand-name">
                <small>{tied.name || 'Jogador'}</small>
                <strong>{categoryLabel(ownCategory) || 'Mesma combinação'}</strong>
              </span>
                <OutcomeCards cards={combinationCards(tied.cards)} startIndex={(index + 1) * 5}/>
              </div>)}
          </div> :
            <OutcomeCards cards={ownCombination} viewerHoleCards={shown.viewerHoleCards || shown.winningHoleCards}/>}
            <p className="hand-outcome-tie-note">Mesma combinação. Os naipes não desempatam.</p>
          {amountDetails && <p className="hand-outcome-tie-note">{amountDetails}</p>}
        </>}
        
        {shown.kind === 'mixed' && <>
            <div className="hand-outcome-heading">
                <Equal aria-hidden="true"/>
                <span><b>Resultado misto</b><small>Você ganhou um pote e perdeu outro</small></span>
            </div>
            <div className="hand-outcome-comparison">
              {shown.pots?.map((potOutcome, index) => {
                const label = shown.pots && shown.pots.length > 1 ? `Pote ${index + 1}` : undefined;
                if (potOutcome.won) return (
                  <div key={index} className="hand-outcome-comparison-row viewer">
                    <span className="hand-outcome-hand-name">
                      <small>{[label, 'Você'].filter(Boolean).join(' · ')}</small>
                      <strong>{categoryLabel(ownCategory) || 'Sua mão'}</strong>
                    </span>
                    <OutcomeCards cards={ownCombination} viewerHoleCards={shown.viewerHoleCards} startIndex={index * 5}/>
                  </div>
                );
                const potCategory = categoryFor(potOutcome.winningCards, potOutcome.category);
                return (
                  <div key={index} className="hand-outcome-comparison-row winner">
                    <span className="hand-outcome-hand-name">
                      <small>{[label, potOutcome.winnerName || 'Vencedor'].filter(Boolean).join(' · ')}</small>
                      <strong>{categoryLabel(potCategory) || 'Mão vencedora'}</strong>
                    </span>
                    <OutcomeCards cards={combinationCards(potOutcome.winningCards, potOutcome.category)}
                                  startIndex={index * 5}/>
                  </div>
                );
              })}
            </div>
          {amountDetails && <p className="hand-outcome-tie-note">{amountDetails}</p>}
        </>}
        
        {shown.kind === 'fold' && <>
            <div className="hand-outcome-heading">
          <span><b>{shown.couldHaveWon ? 'Você poderia ter ganhado!' : 'Você desistiu.'}</b>
            <small>{shown.couldHaveWon ? 'Sua mão batia a mão revelada' : 'Aguardando a próxima mão'}</small></span>
            </div>
          {shown.couldHaveWon && <div className="hand-outcome-comparison">
              <div className="hand-outcome-comparison-row winner">
            <span className="hand-outcome-hand-name">
              <small>{shown.winnerName || 'Vencedor'}</small>
              <strong>{categoryLabel(winnerCategory) || 'Mão revelada'}</strong>
            </span>
                  <OutcomeCards cards={winningCombination} startIndex={0}/>
              </div>
              <span className="hand-outcome-versus">perdia de</span>
              <div className="hand-outcome-comparison-row viewer">
            <span className="hand-outcome-hand-name">
              <small>Sua mão (desistida)</small>
              <strong>{categoryLabel(ownCategory) || 'Sua mão'}</strong>
            </span>
                  <OutcomeCards cards={ownCombination} viewerHoleCards={shown.viewerHoleCards} startIndex={5}/>
              </div>
          </div>}
        </>}
        <OutcomeSettlement pots={shown.resolvedPots} runItTwice={shown.runItTwice}/>
        {shown.kind !== 'fold' && chipChange}
        <small className="hand-outcome-next">A próxima mão já está a caminho.</small>
      </div>
      )}
    </div>
  </>;
}
