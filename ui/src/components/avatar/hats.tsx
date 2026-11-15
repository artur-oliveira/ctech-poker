import type {JSX, ReactNode} from 'react';

/*
 * Avatar hat art (DESIGN.md "Avatar cosmetics art contract"). Same 64x64 box as
 * the frames, avatar circle centred (32,32) r=32. A hat sits ON the head: it
 * covers the top arc of the circle and stays in y 0-26 (declared in data-y-range
 * and checked by the tests). The frame ring (r 25-32) paints over the hat, so
 * each silhouette is built to read inside r≈25 and only brims run under the ring.
 * At a 24px seat one unit is 0.375px: shapes that must read are >=4 units, the
 * 1-1.5 unit lines are contrast edges only (ink on light photos, a pale sheen
 * on the ink hat for dark ones).
 */

type Token = 'gold' | 'gold-pale' | 'oxblood' | 'felt' | 'walnut' | 'paper' | 'ink' | 'wine'
  | 'silver-deep' | 'silver-pale' | 'sapphire' | 'sapphire-deep' | 'sapphire-pale';

const paint = (token: Token) => `var(--cosmetic-${token})`;

/** Pale contrast edge for ink pieces, so they hold on dark photos. */
const SHEEN = {stroke: paint('gold-pale'), strokeWidth: 1, strokeOpacity: 0.5} as const;

function Hat({y, children}: { y: string; children: ReactNode }) {
  return <svg viewBox="0 0 64 64" aria-hidden="true" focusable="false" data-band="hat" data-y-range={y}
    strokeLinejoin="round" strokeLinecap="round">{children}</svg>;
}

/** A filled shape with an ink contrast edge. */
function Shape({d, c, edge = 1.25}: { d: string; c: Token; edge?: number }) {
  return <path d={d} fill={paint(c)} stroke={paint('ink')} strokeWidth={edge}/>;
}

const f = (n: number) => n.toFixed(2);

/** Laurel: leaves along an r=19.5 arc (clear of the frame ring) from the temples up to the crown, open at the top. */
const LAUREL_LEAVES = (() => {
  const R = 19.5, L = 7.5, W = 3.4;
  const leaves: string[] = [];
  for (let i = 0; i < 6; i++) {
    const deg = 203 + i * 10.5;
    const a = (deg * Math.PI) / 180;
    const bx = 32 + R * Math.cos(a), by = 32 + R * Math.sin(a);
    const tangent = a + Math.PI / 2; // towards the crown
    for (const side of [-1, 1]) {
      const dir = tangent + side * 0.62; // one leaf outward, one inward
      const tx = bx + L * Math.cos(dir), ty = by + L * Math.sin(dir);
      const mx = (bx + tx) / 2, my = (by + ty) / 2;
      const nx = -Math.sin(dir) * W, ny = Math.cos(dir) * W;
      leaves.push(`M${f(bx)} ${f(by)} Q${f(mx + nx)} ${f(my + ny)} ${f(tx)} ${f(ty)} Q${f(mx - nx)} ${f(my - ny)} ${f(bx)} ${f(by)} Z`);
    }
  }
  const left = leaves.join(' ');
  // Mirror x -> 64-x for the right branch.
  const right = left.replace(/(-?\d+\.\d+) (-?\d+\.\d+)/g, (_, x, y) => `${f(64 - Number(x))} ${y}`);
  const stem = Array.from({length: 7}, (_, i) => {
    const a = ((203 + i * 9.5) * Math.PI) / 180; // r=R below
    return `${i ? 'L' : 'M'}${f(32 + R * Math.cos(a))} ${f(32 + R * Math.sin(a))}`;
  }).join(' ');
  return {left, right, stem, stemR: stem.replace(/(-?\d+\.\d+) (-?\d+\.\d+)/g, (_, x, y) => `${f(64 - Number(x))} ${y}`)};
})();

export const HAT_ART: Record<string, () => JSX.Element> = {
  // Dealer's celluloid eyeshade: a felt-green crescent across the brow, slightly
  // see-through, bound in gold-pale along the peak's edge.
  'dealer-visor': () => <Hat y="10-26">
    <path d="M7 19 C13 11.5 51 11.5 57 19 C51 25 13 25 7 19 Z" fill={paint('felt')} fillOpacity={0.82}
      stroke={paint('ink')} strokeWidth={1.25}/>
    <path d="M8.5 20.5 C15 24.4 49 24.4 55.5 20.5" fill="none" stroke={paint('gold-pale')} strokeWidth={1.5}/>
    <path d="M10 17 C17 13 47 13 54 17" fill="none" stroke={paint('ink')} strokeWidth={2.5} strokeOpacity={0.55}/>
  </Hat>,
  // Three-point gold crown with a pale top light and one oxblood stone.
  'crown': () => <Hat y="6.5-26">
    <Shape c="gold" d="M17 24.5 L16 11.5 L24.5 18 L32 7.5 L39.5 18 L48 11.5 L47 24.5 Z"/>
    <path d="M17.5 19.75 L46.5 19.75" stroke={paint('gold-pale')} strokeWidth={1.5}/>
    <circle cx={32} cy={22.25} r={2} fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={0.75}/>
  </Hat>,
  // Gold laurel wreath following the head's top arc, open at the crown.
  'laurel': () => <Hat y="5-26">
    <path d={LAUREL_LEAVES.stem} fill="none" stroke={paint('ink')} strokeWidth={2.5}/>
    <path d={LAUREL_LEAVES.stemR} fill="none" stroke={paint('ink')} strokeWidth={2.5}/>
    <path d={LAUREL_LEAVES.left} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.9}/>
    <path d={LAUREL_LEAVES.right} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.9}/>
  </Hat>,
  // Ink top hat with an oxblood band; a pale sheen edge keeps it on dark photos.
  'top-hat': () => <Hat y="5.5-26">
    <path d="M22.5 7 L41.5 7 L42.5 21.5 L21.5 21.5 Z" fill={paint('ink')}
      stroke={paint('gold-pale')} strokeWidth={1} strokeOpacity={0.5}/>
    <rect x={21.9} y={16.5} width={20.2} height={4.5} fill={paint('oxblood')}/>
    <path d="M11 23 C11 20.5 53 20.5 53 23 C53 25 11 25 11 23 Z" fill={paint('ink')}
      stroke={paint('gold-pale')} strokeWidth={1} strokeOpacity={0.5}/>
    <path d="M25.5 9.5 L26 15.5" stroke={paint('paper')} strokeWidth={1.5} strokeOpacity={0.3}/>
  </Hat>,
  // Walnut cowboy hat: pinched crown, gold-pale band, brim curling up at both ends.
  'cowboy': () => <Hat y="7-26">
    <Shape c="walnut" d="M21.5 21.5 C20.5 14 22 9.5 26 9 C29 8.6 30 11 32 11 C34 11 35 8.6 38 9 C42 9.5 43.5 14 42.5 21.5 Z"/>
    <rect x={21.4} y={17.5} width={21.2} height={3} fill={paint('gold-pale')}/>
    <Shape c="walnut" d="M5.5 15.5 C8 22.25 15 24.6 32 24.6 C49 24.6 56 22.25 58.5 15.5 C54 20 46 21.5 32 21.5 C18 21.5 10 20 5.5 15.5 Z"/>
  </Hat>,
  // Wine knit beanie, oxblood turned-up cuff, paper pompom.
  'beanie': () => <Hat y="6-26">
    <Shape c="wine" d="M13 21 C13 13 21 10 32 10 C43 10 51 13 51 21 Z"/>
    <path d="M24 12.5 C21.5 14.5 20.5 17 20.5 20 M32 10.5 L32 20 M40 12.5 C42.5 14.5 43.5 17 43.5 20" fill="none"
      stroke={paint('oxblood')} strokeWidth={1.5} strokeOpacity={0.8}/>
    <rect x={11.5} y={19} width={41} height={6.25} rx={2.5} fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={1.25}/>
    <circle cx={32} cy={10.75} r={4} fill={paint('paper')} stroke={paint('ink')} strokeWidth={1.25}/>
  </Hat>,
  // Grey felt fedora: teardrop crown pinched at the front, ink band, flat brim, one gold-pale feather.
  'fedora': () => <Hat y="7.5-26">
    <Shape c="silver-deep" d="M21 21.5 C20.5 15 21.5 10.5 25 9.5 C28 8.7 29.5 11.5 32 11.5 C34.5 11.5 36 8.7 39 9.5 C42.5 10.5 43.5 15 43 21.5 Z"/>
    <rect x={21.2} y={17.25} width={21.6} height={3.25} fill={paint('ink')}/>
    <path d="M37.5 17.5 L42 13.5" stroke={paint('gold-pale')} strokeWidth={1.5}/>
    <Shape c="silver-deep" d="M7 22.5 C10 20 20 19.75 32 19.75 C44 19.75 54 20 57 22.5 C54 24.75 44 25 32 25 C20 25 10 24.75 7 22.5 Z"/>
  </Hat>,
  // Ink beret worn tilted, oxblood piping at the headband and the little stalk on top; a pale sheen keeps it on dark photos.
  'beret': () => <Hat y="5-26">
    <path d="M33 10.5 L34 6.5" stroke={paint('ink')} strokeWidth={2}/>
    <path d="M10 21 C9 14 19 10 33 10 C46 10 55 13.5 54 19 C53 22.5 47 23.5 32 23.5 C20 23.5 11 24 10 21 Z" fill={paint('ink')} {...SHEEN}/>
    <path d="M14 22 C22 24.25 42 24.25 50 21.5" fill="none" stroke={paint('oxblood')} strokeWidth={2}/>
  </Hat>,
  // Ink bowler: round dome, wine band, short curled brim; a pale sheen keeps it on dark photos.
  'bowler': () => <Hat y="7-26">
    <path d="M20.5 21.5 C20.5 12 25 8 32 8 C39 8 43.5 12 43.5 21.5 Z" fill={paint('ink')} {...SHEEN}/>
    <rect x={20.9} y={17.5} width={22.2} height={3.25} fill={paint('wine')}/>
    <path d="M14 22.5 C14 20.5 50 20.5 50 22.5 C50 24.75 14 24.75 14 22.5 Z" fill={paint('ink')} {...SHEEN}/>
    <path d="M24.5 13.5 C25.5 11 27.5 10 29.5 9.75" fill="none" stroke={paint('paper')} strokeWidth={1.5} strokeOpacity={0.3}/>
  </Hat>,
  // Captain's cap: paper crown, ink band and peak, gold cord and badge.
  'captain-cap': () => <Hat y="8-26">
    <Shape c="paper" d="M15 18 C15 12 23 9 32 9 C41 9 49 12 49 18 L47 20.5 L17 20.5 Z"/>
    <rect x={17} y={18} width={30} height={3.5} fill={paint('ink')}/>
    <path d="M18.5 19.25 L45.5 19.25" stroke={paint('gold')} strokeWidth={1}/>
    <path d="M17.5 21.5 L46.5 21.5 C44 25.25 20 25.25 17.5 21.5 Z" fill={paint('ink')} {...SHEEN}/>
    <circle cx={32} cy={14.5} r={2.5} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.75}/>
  </Hat>,
  // Sapphire wizard's cone with a deep brim and two gold four-point stars.
  'wizard': () => <Hat y="1-26">
    <Shape c="sapphire" d="M20 21.5 L30.5 2.5 C31.25 1.75 32.25 1.75 33 2.75 L44 21.5 Z"/>
    <path d="M30 11 L31 13 L33 14 L31 15 L30 17 L29 15 L27 14 L29 13 Z M36.5 6.5 L37.1 7.9 L38.5 8.5 L37.1 9.1 L36.5 10.5 L35.9 9.1 L34.5 8.5 L35.9 7.9 Z"
      fill={paint('gold')}/>
    <Shape c="sapphire-deep" d="M10 22.5 C10 20 54 20 54 22.5 C54 25 10 25 10 22.5 Z"/>
  </Hat>,
  // Open-top sun visor: paper headband, sapphire bill, a pale sheen along the bill.
  'sun-visor': () => <Hat y="14-26">
    <Shape c="paper" d="M12 19.5 C17 15.5 47 15.5 52 19.5 L51 21 C46 17.75 18 17.75 13 21 Z"/>
    <Shape c="sapphire" d="M13 21 C18 17.75 46 17.75 51 21 C49 23.5 40 25 32 25 C24 25 15 23.5 13 21 Z"/>
    <path d="M18 21.5 C25 19.75 39 19.75 46 21.5" fill="none" stroke={paint('sapphire-pale')} strokeWidth={1.25} strokeOpacity={0.7}/>
  </Hat>,
};
