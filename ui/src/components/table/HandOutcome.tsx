'use client';
import {useEffect, useState} from 'react';
import {PartyPopper} from 'lucide-react';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {PlayingCard} from '@/components/table/PlayingCard';
import {ChipStack} from '@/components/table/ChipStack';
import {useCountUp} from '@/lib/hooks/useCountUp';

export type HandOutcomeState = {
  key: number; kind: 'win' | 'lose'; handCategory?: string; opponentCategory?: string;
  // The winning 5-card hand (or just the 2 hole cards when the board isn't
  // complete) for whoever actually won this pot — the viewer's own on a win,
  // the rival's on a loss — undefined when the hand ended without a
  // showdown, since no one's cards were ever revealed to compare.
  winningCards?: string[];
  // The viewer's stack right before this hand resolved and right after — the
  // chip counter below animates between the two, up when they gained chips,
  // down when they lost some, and stays hidden when neither changed (e.g. a
  // free showdown they simply lost with nothing left in the pot for them).
  stackBefore?: number;
  stackAfter?: number;
};

const EXIT_MS = 320;
const CONFETTI_PIECES = Array.from({length: 8}, (_, i) => i);
const CHIP_COUNT_MS = 700;

type CategoryMeta = { gender: 'm' | 'f'; plural?: boolean };

// Portuguese noun gender/number per category, for the possessive ("Seu" /
// "Sua" / "Seus" / "Suas") and verb agreement in the matchup sentence below.
// Only two_pair is plural ("dois pares"); everything else stays singular.
const CATEGORY_META: Record<string, CategoryMeta> = {
  high_card: {gender: 'f'},
  pair: {gender: 'm'},
  two_pair: {gender: 'm', plural: true},
  three_of_a_kind: {gender: 'f'},
  straight: {gender: 'f'},
  flush: {gender: 'm'},
  full_house: {gender: 'm'},
  four_of_a_kind: {gender: 'f'},
  straight_flush: {gender: 'm'},
  royal_flush: {gender: 'm'}
};

function possessive({gender, plural}: CategoryMeta): string {
  return plural ? (gender === 'f' ? 'Suas' : 'Seus') : (gender === 'f' ? 'Sua' : 'Seu');
}

function agree(singular: string, plural: string, meta: CategoryMeta): string {
  return meta.plural ? plural : singular;
}

// One sentence naming the actual matchup instead of a bare category chip:
// same category on both sides needs a tie-break line (the category alone
// doesn't explain who won); different categories name both hands directly.
// `seed` (the outcome's ever-increasing key) rotates through a few phrasings
// so a player on a winning or losing streak doesn't read the same line twice
// in a row.
function describeMatchup(kind: 'win' | 'lose', ownKey?: string, rivalKey?: string, seed = 0): string | null {
  if (!ownKey || !rivalKey) return null;
  const own = CATEGORY_META[ownKey] || {gender: 'm'};
  const ownLower = (HAND_CATEGORY_LABELS[ownKey] || ownKey).toLowerCase();
  const rivalLower = (HAND_CATEGORY_LABELS[rivalKey] || rivalKey).toLowerCase();
  const poss = possessive(own);

  if (ownKey === rivalKey) {
    const variants = kind === 'win' ? [
      `${poss} ${ownLower} é ${agree('maior', 'maiores', own)} ${ownLower}.`,
      `${poss} ${ownLower} ${agree('é', 'são', own)} ${agree('o', 'os', own)} ` +
      `${agree('melhor', 'melhores', own)} da mesa no desempate.`
    ] : [
      `${poss} ${ownLower} não ${agree('vence', 'vencem', own)} de ${rivalLower}.`,
      `${poss} ${ownLower} ${agree('perdeu', 'perderam', own)} no desempate.`
    ];
    return variants[seed % variants.length];
  }

  const ownLabel = HAND_CATEGORY_LABELS[ownKey] || ownKey;
  const variants = kind === 'win' ? [
    `${ownLabel} ganha em cima de ${rivalLower}.`,
    `${ownLabel} supera ${rivalLower} com folga.`,
    `${ownLabel} leva a melhor sobre ${rivalLower}.`,
    `${ownLabel} fecha na frente de ${rivalLower}.`
  ] : [
    `${poss} ${ownLower} não ganha de ${rivalLower}.`,
    `${poss} ${ownLower} não vence ${rivalLower}.`,
    `${poss} ${ownLower} fica atrás de ${rivalLower}.`,
    `${poss} ${ownLower} não é páreo para ${rivalLower}.`
  ];
  return variants[seed % variants.length];
}

/** Three-beat reveal of a stack change: the stack as it was, the delta that's
 * about to land, then the two merging into one counted total — counting up
 * on a gain, down on a loss, since the same sequence reads honestly either
 * way. Skips straight to the merged total under reduced motion instead of
 * dropping the animation silently — the number itself still has to end up
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
 * caller bumps only when payouts first appear for that hand) — never on the
 * repeat broadcasts that follow while the table sits in `complete`. Purely
 * decorative: the sr-only live region already announces the same outcome.
 *
 * Stays open for as long as the table itself is still showing that resolved
 * hand's payouts (`holdOpen`) instead of a fixed timer — a player who glances
 * away mid-hand and looks back a few seconds later still finds their
 * win/loss on screen, not a banner that already auto-dismissed under them.
 * It closes once the next hand actually starts. */
export function HandOutcomeBanner({outcome, holdOpen}: { outcome: HandOutcomeState | null; holdOpen: boolean }) {
  const [shown, setShown] = useState(outcome);
  const [seenKey, setSeenKey] = useState(outcome?.key);

  if (outcome && outcome.key !== seenKey) {
    setSeenKey(outcome.key);
    setShown(outcome);
  }

  const leaving = !!shown && !holdOpen;

  useEffect(() => {
    if (!leaving) return () => {
    };
    const clear = setTimeout(() => setShown(null), EXIT_MS);
    return () => clearTimeout(clear);
  }, [leaving]);

  if (!shown) return null;
  const category = shown.handCategory && (HAND_CATEGORY_LABELS[shown.handCategory] || shown.handCategory);
  const detail = describeMatchup(shown.kind, shown.handCategory, shown.opponentCategory, shown.key);
  return <div className="hand-outcome" aria-hidden="true">
    <div key={shown.key} className={`hand-outcome-card ${shown.kind}${leaving ? ' leaving' : ''}`}>
      {shown.kind === 'win' && <span className="hand-outcome-confetti">{CONFETTI_PIECES.map(i =>
        <span key={i}/>)}</span>}
      {shown.kind === 'win' ? <PartyPopper/> : null}
      <b>{shown.kind === 'win' ? 'Você venceu a mão!' : 'Não foi dessa vez.'}</b>
      {shown.winningCards && <span className="hand-outcome-cards">
        {shown.winningCards.map((card, i) => <PlayingCard key={i} card={card} index={i} size="hole"
                                                            owner={shown.kind === 'win' ? 'viewer' : 'opponent'}/>)}
      </span>}
      {shown.stackBefore != null && shown.stackAfter != null && shown.stackBefore !== shown.stackAfter &&
          <ChipCountUp from={shown.stackBefore} to={shown.stackAfter}/>}
      {detail ? <p className="hand-outcome-detail">{detail}</p> : category && <small>{category}</small>}
      {shown.kind === 'lose' && <small>A próxima mão já está a caminho.</small>}
    </div>
  </div>;
}
