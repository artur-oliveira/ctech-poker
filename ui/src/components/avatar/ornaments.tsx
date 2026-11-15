import type {JSX, ReactNode} from 'react';

/*
 * Avatar ornament art, the accessory slot (DESIGN.md "Avatar cosmetics art
 * contract"). Two bands, both in the frames' 64-unit box (avatar circle at
 * 32,32, r=32):
 * - wings sit BEHIND the avatar: `PlayerAvatar` paints them under the photo,
 *   so only what spreads past the circle shows. Their viewBox is the box plus
 *   18 units each side and 10 above (`WINGS_VIEWBOX`).
 * - horns sit ON the avatar like a hat (same rise, `HORNS_VIEWBOX` is the hat
 *   zone plus 8 units each side for the bull's sweep), painted after the frame
 *   and before the hat, so a hat covers them. Nothing paints inside r=28.5.
 * Flat like the hats (no gradients or transforms; the right half is the left
 * one mirrored in code). At a 24px seat one unit is 0.375px, so every piece
 * that must read on its own is a big shape (feathers ~7 units wide), and 1-unit
 * lines are only contrast edges.
 */

export const WINGS_VIEWBOX = [-18, -10, 100, 64] as const;
export const HORNS_VIEWBOX = [-8, -22, 80, 36] as const;

export type OrnamentBand = 'wings' | 'horns';

/** How far above the avatar box each band rises, as a fraction of the avatar size. */
export const ORNAMENT_RISE: Record<OrnamentBand, number> = {wings: -WINGS_VIEWBOX[1] / 64, horns: -HORNS_VIEWBOX[1] / 64};

/** How far each band reaches past each side of the avatar box, as a fraction of its size. */
export const ORNAMENT_SPREAD: Record<OrnamentBand, number> = {wings: -WINGS_VIEWBOX[0] / 64, horns: -HORNS_VIEWBOX[0] / 64};
export const WING_SPREAD = ORNAMENT_SPREAD.wings;

type Token = 'gold' | 'gold-pale' | 'gold-deep' | 'oxblood' | 'oxblood-bright' | 'wine' | 'paper' | 'ink'
  | 'pearl-shade' | 'walnut' | 'walnut-deep' | 'obsidian' | 'obsidian-lift' | 'silver-pale' | 'silver-deep'
  | 'emerald' | 'emerald-deep';
type P = [number, number];

const paint = (token: Token) => `var(--cosmetic-${token})`;
const f = (n: number) => n.toFixed(2);
const pt = ([x, y]: P) => `${f(x)} ${f(y)}`;
const lerp = (a: P, b: P, t: number): P => [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t];
/** The left half's path mirrored onto the right (every "x y" pair, absolute commands only). */
const mirror = (d: string) => d.replace(/(-?\d*\.?\d+) (-?\d*\.?\d+)/g, (_, x, y) => `${f(64 - Number(x))} ${y}`);

function Ornament({band, children}: { band: OrnamentBand; children: ReactNode }) {
  const box = band === 'wings' ? WINGS_VIEWBOX : HORNS_VIEWBOX;
  return <svg viewBox={box.join(' ')} aria-hidden="true" focusable="false" data-band={band}
    strokeLinejoin="round" strokeLinecap="round">{children}</svg>;
}

/** A path drawn on both sides: the left as given, the right mirrored. */
function Pair({d, ...rest}: { d: string } & Record<string, string | number>) {
  return <><path d={d} {...rest}/><path d={mirror(d)} {...rest}/></>;
}

/** A feather from root r to tip t, `w` wide near its rounded tip. */
function feather(r: P, t: P, w: number) {
  const len = Math.hypot(t[0] - r[0], t[1] - r[1]);
  const n: P = [-(t[1] - r[1]) / len * w, (t[0] - r[0]) / len * w];
  const a = lerp(r, t, 0.3), b = lerp(r, t, 0.85);
  return `M${pt(r)} C${pt([a[0] + n[0], a[1] + n[1]])} ${pt([b[0] + n[0], b[1] + n[1]])} ${pt(t)}`
    + ` C${pt([b[0] - n[0], b[1] - n[1]])} ${pt([a[0] - n[0], a[1] - n[1]])} ${pt(r)} Z`;
}

/** Left wing of feathers: six long primaries fanned from behind the photo, a
 * shorter row of secondaries over them in the second tone. */
const FEATHERS = (() => {
  const root: P = [12, 30];
  const tips: P[] = [[-2, -6.5], [-11, -1.5], [-16, 8], [-16, 19], [-12.5, 29.5], [-6, 38.5]];
  const primaries = tips.map(t => feather(root, t, 5.2)).join(' ');
  const secondaries = tips.slice(1).map((t, i) => feather(root, lerp(root, lerp(tips[i], t, 0.5), 0.72), 5)).join(' ');
  return {primaries, secondaries};
})();

function FeatherWings({outer, inner}: { outer: Token; inner: Token }) {
  return <Ornament band="wings">
    <Pair d={FEATHERS.primaries} fill={paint(outer)} stroke={paint('ink')} strokeWidth={1}/>
    <Pair d={FEATHERS.secondaries} fill={paint(inner)} stroke={paint('ink')} strokeWidth={0.9}/>
  </Ornament>;
}

/** A membrane wing (left): leading edge root→wrist, fingers wrist→tips, a
 * scalloped trailing edge between the tips back to the body. */
function membrane(root: P, wrist: P, tips: P[], body: P) {
  const ends = [...tips, body];
  const scallop = (a: P, b: P) => `Q${pt(lerp(lerp(a, b, 0.5), wrist, 0.3))} ${pt(b)}`;
  const outline = `M${pt(root)} L${pt(wrist)} L${pt(tips[0])} `
    + ends.slice(1).map((e, i) => scallop(ends[i], e)).join(' ') + ' Z';
  // Every other panel between two fingers, for the second tone.
  const panels = tips.slice(0, -1).filter((_, i) => i % 2 === 1)
    .map(t => { const i = tips.indexOf(t); return `M${pt(wrist)} L${pt(t)} ${scallop(t, tips[i + 1])} Z`; }).join(' ');
  const bones = tips.map(t => `M${pt(wrist)} L${pt(t)}`).join(' ');
  return {outline, panels, bones, arm: `M${pt(root)} L${pt(wrist)}`};
}

const BAT = membrane([14, 24], [-4, -5], [[-17, 4], [-17, 22], [-9, 37]], [12, 44]);
const DRAGON = membrane([14, 22], [-6, -7], [[-17, 2], [-16.8, 17], [-12.5, 31], [-3, 42]], [12, 46]);

export const ORNAMENTS: Record<string, { band: OrnamentBand; Art: () => JSX.Element }> = {
  // Paper feathers over pearl primaries, ink-edged so they hold on a light card.
  'angel-wings': {band: 'wings', Art: () => <FeatherWings outer="pearl-shade" inner="paper"/>},
  // Obsidian membrane, a lifted panel, silver-deep fingers and a pale claw.
  'bat-wings': {band: 'wings', Art: () => <Ornament band="wings">
    <Pair d={BAT.outline} fill={paint('obsidian')} stroke={paint('silver-pale')} strokeWidth={1} strokeOpacity={0.75}/>
    <Pair d={BAT.panels} fill={paint('obsidian-lift')}/>
    <Pair d={BAT.bones} fill="none" stroke={paint('silver-deep')} strokeWidth={1.5}/>
    <Pair d={BAT.arm} fill="none" stroke={paint('obsidian-lift')} strokeWidth={2.6}/>
    <Pair d="M-4 -5 Q-5 -8 -2 -8.8" fill="none" stroke={paint('silver-pale')} strokeWidth={1.4}/>
  </Ornament>},
  // Gold-pale feathers over gold primaries.
  'gold-wings': {band: 'wings', Art: () => <FeatherWings outer="gold" inner="gold-pale"/>},
  // Emerald membrane with deep panels, gold bones and a spiked leading edge.
  'dragon-wings': {band: 'wings', Art: () => <Ornament band="wings">
    <Pair d={DRAGON.outline} fill={paint('emerald')} stroke={paint('ink')} strokeWidth={1}/>
    <Pair d={DRAGON.panels} fill={paint('emerald-deep')}/>
    <Pair d={DRAGON.bones} fill="none" stroke={paint('gold-deep')} strokeWidth={1.6}/>
    <Pair d="M3 9 L1 3.5 L5.5 6.5 Z M-1.5 3 L-4 -2 L0.5 0.5 Z M-6 -7 L-6.5 -9.6 L-3.5 -8 Z" fill={paint('gold')}
      stroke={paint('ink')} strokeWidth={0.6}/>
    <Pair d={DRAGON.arm} fill="none" stroke={paint('gold')} strokeWidth={2.4}/>
  </Ornament>},

  // Short oxblood horns curving up from the crown, a bright ridge on each.
  'devil-horns': {band: 'horns', Art: () => <Ornament band="horns">
    <Pair d="M12.5 7.8 C9.5 1 8.5 -7 10.5 -13 C13 -8 17 -4 21 3.15 Z" fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={1.25}/>
    <Pair d="M13.4 4.6 C12.2 -0.5 11.6 -5 11.4 -8.8" fill="none" stroke={paint('oxblood-bright')} strokeWidth={1.5} strokeOpacity={0.85}/>
  </Ornament>},
  // Walnut ram horns curling back and down at the temples, ridged.
  'ram-horns': {band: 'horns', Art: () => {
    const curl = 'M15 4.6 C12 -4 4 -8 -1 -4 C-4.4 -1 -4.4 6 -1 8 C2 9.5 4.5 6 2.5 3.5';
    return <Ornament band="horns">
      <Pair d={curl} fill="none" stroke={paint('ink')} strokeWidth={7}/>
      <Pair d={curl} fill="none" stroke={paint('walnut')} strokeWidth={5}/>
      <Pair d={curl} fill="none" stroke={paint('walnut-deep')} strokeWidth={5} strokeDasharray="1.1 2.2" strokeLinecap="butt"/>
    </Ornament>;
  }},
  // Ivory bull horns sweeping out past the temples and up, dark tips.
  'bull-horns': {band: 'horns', Art: () => <Ornament band="horns">
    <Pair d="M10 10.26 C2 10.5 -5.5 6 -6.5 -2 C-7 -7 -6.5 -11 -5 -14 C-3 -9 -1.5 -4 2 -1 C6 2.5 12 3.5 18.5 4.49 Z"
      fill={paint('pearl-shade')} stroke={paint('ink')} strokeWidth={1.25}/>
    <Pair d="M-6.6 -4 C-7 -8 -6.5 -11 -5 -14 C-3.6 -10.5 -2.6 -7.5 -1.2 -5 Z" fill={paint('walnut-deep')}/>
  </Ornament>},
};

/** id → art, the shape the store and profile pickers take for every slot. */
export const ORNAMENT_ART: Record<string, () => JSX.Element> =
  Object.fromEntries(Object.entries(ORNAMENTS).map(([id, {Art}]) => [id, Art]));
