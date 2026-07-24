'use client';
import {useEffect, useState} from 'react';
import {PartyPopper} from 'lucide-react';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';

export type HandOutcomeState = {
  key: number; kind: 'win' | 'lose'; amount: number; handCategory?: string; opponentCategory?: string
};

const EXIT_MS = 320;
const CONFETTI_PIECES = Array.from({length: 8}, (_, i) => i);

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

/** Fires once per resolved hand (keyed by an ever-increasing counter the
 * caller bumps only when payouts first appear for that hand) — never on the
 * repeat broadcasts that follow while the table sits in `complete`. Purely
 * decorative: the sr-only live region already announces the same outcome.
 *
 * Stays open for as long as the table itself is still on that resolved hand
 * (`holdOpen`, driven by `snapshot.stage === 'complete'`) instead of a fixed
 * timer — a player who glances away mid-hand and looks back a few seconds
 * later still finds their win/loss on screen, not a banner that already
 * auto-dismissed under them. It closes once the next hand actually starts. */
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
      {shown.kind === 'win' &&
          <span className="hand-outcome-amount">+{shown.amount.toLocaleString('pt-BR')} fichas</span>}
      {detail ? <p className="hand-outcome-detail">{detail}</p> : category && <small>{category}</small>}
      {shown.kind === 'lose' && <small>A próxima mão já está a caminho.</small>}
    </div>
  </div>;
}
