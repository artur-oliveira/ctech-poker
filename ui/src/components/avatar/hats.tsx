import type {JSX, ReactNode} from 'react';

/*
 * Avatar hat art (DESIGN.md "Avatar cosmetics art contract"). A hat sits ON the
 * avatar circle like a hat on a head seen from the front: its band or brim rests
 * on the circle's top arc, overlapping the ring by ~2 units, and the body rises
 * ABOVE the avatar box. The photo stays uncovered (nothing paints inside r=28.5,
 * checked by the tests). Units are the frames' 64x64 box, avatar circle at
 * (32,32) r=32; the hat's own viewBox runs from y=-22 (0.34 of the avatar above
 * the box) to y=14 (the laurel's sides), and `.player-avatar-art >
 * svg[data-band=hat]` in renderer.css places it on that box. Painted after the
 * frame, so the brim sits on the ring. At a 24px seat one unit is 0.375px:
 * shapes that must read are >=4 units, 1-1.5 unit lines are contrast edges
 * only (ink on light photos, a pale sheen on ink hats for dark ones).
 */

/** x, y, width, height of every hat's viewBox, in the 64-box's units. */
export const HAT_VIEWBOX = [0, -22, 64, 36] as const;

/** How far above the avatar box a hat may rise, as a fraction of the avatar size. */
export const HAT_RISE = -HAT_VIEWBOX[1] / 64;

type Token = 'gold' | 'gold-pale' | 'gold-deep' | 'oxblood' | 'felt' | 'walnut' | 'paper' | 'ink' | 'wine'
  | 'silver-deep' | 'silver-pale' | 'sapphire' | 'sapphire-deep' | 'sapphire-pale' | 'walnut-deep' | 'obsidian-lift'
  | 'emerald-pale';

const paint = (token: Token) => `var(--cosmetic-${token})`;
const f = (n: number) => n.toFixed(2);

/** Pale contrast edge for ink pieces, so they hold on dark photos and the dark theme. */
const SHEEN = {stroke: paint('silver-pale'), strokeWidth: 1.25, strokeOpacity: 0.8} as const;

function Hat({children}: { children: ReactNode }) {
  return <svg viewBox={HAT_VIEWBOX.join(' ')} aria-hidden="true" focusable="false" data-band="hat"
    strokeLinejoin="round" strokeLinecap="round">{children}</svg>;
}

/** A filled shape with an ink contrast edge. */
function Shape({d, c, edge = 1.25}: { d: string; c: Token; edge?: number }) {
  return <path d={d} fill={paint(c)} stroke={paint('ink')} strokeWidth={edge}/>;
}

/** The circle's top edge at x. */
const head = (x: number) => 32 - Math.sqrt(1024 - (x - 32) ** 2);

/** Quadratic through (x, head(x)+dy), (32, dy) and the mirror point: the head's
 * top arc from x to 64-x, shifted down by dy. `to` draws it right to left. */
const arc = (x: number, dy: number, to: 'right' | 'left' = 'right') => {
  const y = head(x) + dy, c = dy - head(x);
  return to === 'right' ? `M${f(x)} ${f(y)} Q32 ${f(c)} ${f(64 - x)} ${f(y)}` : `Q32 ${f(c)} ${f(x)} ${f(y)}`;
};

/** A band hugging the head from x to 64-x, from dy=top to dy=bottom. */
const band = (x: number, top: number, bottom: number) =>
  `${arc(x, bottom)} L${f(64 - x)} ${f(head(x) + top)} ${arc(x, top, 'left')} Z`;

/** A brim from x to 64-x: ends at yEnd, middle resting at yMid, `thick` deep. */
const brim = (x: number, yEnd: number, yMid: number, thick: number) => {
  const c = 2 * yMid - yEnd;
  return `M${f(x)} ${f(yEnd)} Q32 ${f(c)} ${f(64 - x)} ${f(yEnd)} L${f(64 - x)} ${f(yEnd - thick)}`
    + ` Q32 ${f(c - thick)} ${f(x)} ${f(yEnd - thick)} Z`;
};

/** Laurel: leaf pairs along an r=35.5 arc outside the ring, temples to near the crown, open at the top. */
const LAUREL = (() => {
  const R = 35.5, L = 6.5, W = 2.6;
  const leaves: string[] = [];
  const stem: string[] = [];
  for (let i = 0; i < 7; i++) {
    const a = ((219 + i * 6.5) * Math.PI) / 180;
    const bx = 32 + R * Math.cos(a), by = 32 + R * Math.sin(a);
    stem.push(`${i ? 'L' : 'M'}${f(bx)} ${f(by)}`);
    const tangent = a + Math.PI / 2; // towards the crown
    for (const side of [-1, 1]) {
      const dir = tangent + side * 0.6; // one leaf outward, one inward
      const tx = bx + L * Math.cos(dir), ty = by + L * Math.sin(dir);
      const mx = (bx + tx) / 2, my = (by + ty) / 2;
      const nx = -Math.sin(dir) * W, ny = Math.cos(dir) * W;
      leaves.push(`M${f(bx)} ${f(by)} Q${f(mx + nx)} ${f(my + ny)} ${f(tx)} ${f(ty)} Q${f(mx - nx)} ${f(my - ny)} ${f(bx)} ${f(by)} Z`);
    }
  }
  const mirror = (d: string) => d.replace(/(-?\d+\.\d+) (-?\d+\.\d+)/g, (_, x, y) => `${f(64 - Number(x))} ${y}`);
  const left = leaves.join(' '), stemL = stem.join(' ');
  return {left, right: mirror(left), stemL, stemR: mirror(stemL)};
})();

export const HAT_ART: Record<string, () => JSX.Element> = {
  // Dealer's celluloid eyeshade: a gold headband on the head and a big see-through
  // felt-green bill tipped up above it, bound in gold-pale along its edge.
  'dealer-visor': () => <Hat>
    <path d={`${arc(8, 1)} L56 ${f(head(8) - 1)} Q32 -20.5 8 ${f(head(8) - 1)} Z`} fill={paint('felt')} fillOpacity={0.9}
      stroke={paint('ink')} strokeWidth={1.25}/>
    <path d="M14 0 Q32 -12 50 0" fill="none" stroke={paint('emerald-pale')} strokeWidth={1.25} strokeOpacity={0.6}/>
    <path d={`M8.75 ${f(head(8) - 1.75)} Q32 -19 55.25 ${f(head(8) - 1.75)}`} fill="none" stroke={paint('gold-pale')} strokeWidth={2}/>
    <Shape c="gold" edge={1} d={band(11, -2, 2.25)}/>
  </Hat>,
  // Three-point gold crown on a gold band, pale balls on the tips, one oxblood stone.
  'crown': () => <Hat>
    <Shape c="gold" d="M17 0 L15 -13.5 L24.25 -6.5 L32 -17 L39.75 -6.5 L49 -13.5 L47 0 Z"/>
    <path d="M24.25 -6.5 L32 -17 L39.75 -6.5" fill="none" stroke={paint('gold-pale')} strokeWidth={1.25}/>
    <Shape c="gold-deep" d={band(15.5, -4.5, 2.25)}/>
    {[15, 32, 49].map(x => <circle key={x} cx={x} cy={x === 32 ? -18.25 : -14.75} r={2.1}
      fill={paint('gold-pale')} stroke={paint('ink')} strokeWidth={0.9}/>)}
    <circle cx={32} cy={-1.5} r={2.1} fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={0.75}/>
  </Hat>,
  // Gold laurel wreath hugging the top of the ring from the outside, open at the crown.
  'laurel': () => <Hat>
    <path d={LAUREL.stemL} fill="none" stroke={paint('gold-deep')} strokeWidth={2}/>
    <path d={LAUREL.stemR} fill="none" stroke={paint('gold-deep')} strokeWidth={2}/>
    <path d={LAUREL.left} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.9}/>
    <path d={LAUREL.right} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.9}/>
  </Hat>,
  // Ink top hat: a tall, slightly flared crown with an oxblood band on a thin curved brim;
  // a silver-pale sheen edge keeps the silhouette on the dark table.
  'top-hat': () => <Hat>
    <path d="M20.5 1 L19.5 -19.5 Q32 -21 44.5 -19.5 L43.5 1 Z" fill={paint('ink')} {...SHEEN}/>
    <path d="M20.25 -4.75 Q32 -5.75 43.75 -4.75 L43.6 -1 Q32 -2 20.4 -1 Z" fill={paint('oxblood')}/>
    <path d="M24 -17 L24.5 -8" stroke={paint('paper')} strokeWidth={1.5} strokeOpacity={0.35}/>
    <path d={brim(11, 3.25, 0.75, 3)} fill={paint('ink')} {...SHEEN}/>
  </Hat>,
  // Walnut cowboy hat: pinched crown, gold-pale band, a wide brim curling up at both ends.
  'cowboy': () => <Hat>
    <Shape c="walnut" d="M20.5 1 C19.5 -7 20.5 -14 25.5 -14.5 C29 -14.8 30 -11.5 32 -11.5 C34 -11.5 35 -14.8 38.5 -14.5 C43.5 -14 44.5 -7 43.5 1 Z"/>
    <path d="M20.1 -4.25 L43.9 -4.25 L43.75 -1.25 L20.25 -1.25 Z" fill={paint('gold-pale')}/>
    <Shape c="walnut" d="M4.5 -6.5 Q8 1.5 32 2.5 Q56 1.5 59.5 -6.5 Q54 -1.5 32 -1.5 Q10 -1.5 4.5 -6.5 Z"/>
  </Hat>,
  // Wine knit beanie with oxblood ribs, an oxblood turned-up cuff and a paper pompom.
  'beanie': () => <Hat>
    <Shape c="wine" d={`M12.75 ${f(head(12.75) - 2)} C12.75 -8 20 -13.5 32 -13.5 C44 -13.5 51.25 -8 51.25 ${f(head(12.75) - 2)} ${arc(12.75, -2, 'left')} Z`}/>
    <path d="M23 -10.75 C20.75 -8 19.75 -5 19.75 -2 M32 -12.75 L32 -3 M41 -10.75 C43.25 -8 44.25 -5 44.25 -2" fill="none"
      stroke={paint('oxblood')} strokeWidth={1.5} strokeOpacity={0.85}/>
    <Shape c="oxblood" d={band(11.5, -4.5, 2.25)}/>
    <circle cx={32} cy={-15.5} r={4.5} fill={paint('paper')} stroke={paint('ink')} strokeWidth={1.25}/>
  </Hat>,
  // Grey felt fedora: teardrop crown pinched at the front, ink band, gold-pale feather, snap brim.
  'fedora': () => <Hat>
    <Shape c="silver-deep" d="M20 1 C19.5 -5.5 20.5 -11.5 24.5 -12.5 C28 -13.3 29.5 -10 32 -10 C34.5 -10 36 -13.3 39.5 -12.5 C43.5 -11.5 44.5 -5.5 44 1 Z"/>
    <path d="M19.9 -4.5 L44.1 -4.5 L44 -1 L20 -1 Z" fill={paint('ink')}/>
    <path d="M38.5 -4 L43.5 -9.5" stroke={paint('gold-pale')} strokeWidth={1.5}/>
    <Shape c="silver-deep" d={brim(8, 1.75, 1, 3)}/>
  </Hat>,
  // Oxblood beret worn tilted to the right, ink piping on the headband and a stalk on top.
  'beret': () => <Hat>
    <path d="M34.5 -10 L35.5 -14" stroke={paint('ink')} strokeWidth={2}/>
    <path d={`M14 ${f(head(14) + 1.5)} C9 -2 14 -10.5 33 -10.5 C49 -10.5 58 -6 57 -1.5 C56.5 1 54 2.5 50 ${f(head(14) + 1.5)} ${arc(14, 1.5, 'left')} Z`}
      fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={1.25}/>
    <path d="M18 -5 C22 -8.5 30 -9.25 38 -9" fill="none" stroke={paint('paper')} strokeWidth={1.5} strokeOpacity={0.3}/>
    <path d={arc(15.5, 1)} fill="none" stroke={paint('ink')} strokeWidth={2}/>
  </Hat>,
  // Brown derby bowler: round walnut dome, ink band, short brim curled up at the ends.
  'bowler': () => <Hat>
    <Shape c="walnut-deep" d="M19.5 1 C19.5 -9 24 -15 32 -15 C40 -15 44.5 -9 44.5 1 Z"/>
    <path d="M19.6 -4.25 L44.4 -4.25 L44.45 -1 L19.55 -1 Z" fill={paint('ink')}/>
    <path d="M25 -9 C25.75 -11.5 27.75 -12.75 29.75 -13" fill="none" stroke={paint('paper')} strokeWidth={1.5} strokeOpacity={0.35}/>
    <Shape c="walnut-deep" d={brim(13, 0.75, 1, 2.75)}/>
  </Hat>,
  // Captain's cap: paper crown flaring to a flat top, ink band and peak, gold cord and badge.
  'captain-cap': () => <Hat>
    <Shape c="paper" d="M17.5 0 C14 -3 11 -7 11.5 -9.5 C12 -12 21 -13.5 32 -13.5 C43 -13.5 52 -12 52.5 -9.5 C53 -7 50 -3 46.5 0 Z"/>
    <Shape c="ink" edge={1} d={band(17, -4.25, 0.25)}/>
    <path d={arc(18.5, -2)} fill="none" stroke={paint('gold')} strokeWidth={1}/>
    <path d={`${arc(17.5, 0)} L46.5 ${f(head(17.5) + 1)} Q32 1 17.5 ${f(head(17.5) + 1)} Z`} fill={paint('ink')} {...SHEEN}/>
    <circle cx={32} cy={-8.5} r={2.6} fill={paint('gold')} stroke={paint('ink')} strokeWidth={0.75}/>
  </Hat>,
  // Sapphire wizard's cone, its tip bent to the right, two gold four-point stars, a deep brim.
  'wizard': () => <Hat>
    <Shape c="sapphire" d="M20.5 1 C25 -6 29 -14 33.5 -19.5 C34.5 -20.75 36 -20.5 36.75 -19.25 L38.5 -16.5 C38 -11 40 -4 43.5 1 Z"/>
    <path d="M30 -8 L31 -6 L33 -5 L31 -4 L30 -2 L29 -4 L27 -5 L29 -6 Z M36.25 -13.5 L36.85 -12.1 L38.25 -11.5 L36.85 -10.9 L36.25 -9.5 L35.65 -10.9 L34.25 -11.5 L35.65 -12.1 Z"
      fill={paint('gold')}/>
    <Shape c="sapphire-deep" d={brim(10, 2, 0.75, 3)}/>
  </Hat>,
  // Open-top sun visor: a paper headband and a big opaque sapphire bill tipped up, pale sheen along it.
  'sun-visor': () => <Hat>
    <Shape c="sapphire" d={`${arc(9, 0.5)} L55 ${f(head(9) - 0.5)} C50 -9 40 -11 32 -11 C24 -11 14 -9 9 ${f(head(9) - 0.5)} Z`}/>
    <path d="M14.5 1 C20 -5.5 44 -5.5 49.5 1" fill="none" stroke={paint('sapphire-pale')} strokeWidth={1.25} strokeOpacity={0.75}/>
    <Shape c="paper" d={band(11, -1.75, 2.25)}/>
  </Hat>,
};
