import type {JSX, ReactNode} from 'react';

/*
 * Avatar accessory art (DESIGN.md "Avatar cosmetics art contract"). Same 64x64
 * box, avatar circle centred (32,32) r=32; accessories paint under the hat and
 * the frame. They live on the eye line (y 24-40, eyes near y 30 at x 24/40), the
 * mustache on the mouth line (y 40-48); the headset's band arcs over the head
 * (hat zone) down to one ear cup on the eye line. Each piece is thin and flat so
 * the face stays visible. Ink pieces carry a faint gold-pale edge so they hold
 * on dark photos; at 24px one unit is 0.375px, so nothing that must read on its
 * own is under 4 units.
 */

type Token = 'gold' | 'gold-pale' | 'oxblood' | 'felt' | 'walnut' | 'paper' | 'ink' | 'wine';

const paint = (token: Token) => `var(--cosmetic-${token})`;

/** Pale contrast edge for ink pieces. */
const SHEEN = {stroke: paint('gold-pale'), strokeWidth: 1, strokeOpacity: 0.5} as const;

function Accessory({y, children}: { y: string; children: ReactNode }) {
  return <svg viewBox="0 0 64 64" aria-hidden="true" focusable="false" data-band="accessory" data-y-range={y}
    strokeLinejoin="round" strokeLinecap="round">{children}</svg>;
}

/** Closed ellipse as cubic Beziers (absolute commands), for even-odd holes. */
function ellipse(cx: number, cy: number, rx: number, ry: number) {
  const k = 0.5523, ox = rx * k, oy = ry * k;
  return `M${cx - rx} ${cy} C${cx - rx} ${cy - oy} ${cx - ox} ${cy - ry} ${cx} ${cy - ry} `
    + `C${cx + ox} ${cy - ry} ${cx + rx} ${cy - oy} ${cx + rx} ${cy} `
    + `C${cx + rx} ${cy + oy} ${cx + ox} ${cy + ry} ${cx} ${cy + ry} `
    + `C${cx - ox} ${cy + ry} ${cx - rx} ${cy + oy} ${cx - rx} ${cy} Z`;
}

export const ACCESSORY_ART: Record<string, () => JSX.Element> = {
  // Ink eye mask across the brow line with two almond eye holes.
  'bandit-mask': () => <Accessory y="24-38">
    <path fillRule="evenodd" fill={paint('ink')} {...SHEEN}
      d={`M7 28.5 C14 25 50 25 57 28.5 L56 33.5 C48 37 40 33.5 32 35 C24 33.5 16 37 8 33.5 Z ${ellipse(23.5, 30.5, 4.5, 2.75)} ${ellipse(40.5, 30.5, 4.5, 2.75)}`}/>
  </Accessory>,
  // Flat ink wayfarer lenses, thin bridge and temples, one gold-pale glint each.
  'shades': () => <Accessory y="25-37">
    <path d="M8 28.5 L15 28 M49 28 L56 28.5" stroke={paint('ink')} strokeWidth={2}/>
    <path d="M30 29 C31.3 28 32.7 28 34 29" fill="none" stroke={paint('ink')} strokeWidth={1.75}/>
    <path d="M14.5 27 L30.5 27 L29.5 33 C29 35.5 27 36 23.5 36 C19 36 16 35 15.25 32.5 Z" fill={paint('ink')} {...SHEEN}/>
    <path d="M33.5 27 L49.5 27 L48.75 32.5 C48 35 45 36 40.5 36 C37 36 35 35.5 34.5 33 Z" fill={paint('ink')} {...SHEEN}/>
    <path d="M18 29 L20.5 29 M37 29 L39.5 29" stroke={paint('gold-pale')} strokeWidth={1.25} strokeOpacity={0.7}/>
  </Accessory>,
  // A single gold ring over the right eye, lightly glazed, inked on both edges.
  'monocle': () => <Accessory y="24-40">
    <circle cx={40.5} cy={31.75} r={5.25} fill={paint('paper')} fillOpacity={0.12}/>
    <circle cx={40.5} cy={31.75} r={5.75} fill="none" stroke={paint('ink')} strokeWidth={4}/>
    <circle cx={40.5} cy={31.75} r={5.75} fill="none" stroke={paint('gold')} strokeWidth={2.5}/>
    <path d="M37.5 28.5 C38.2 27.9 38.9 27.6 39.75 27.5" fill="none" stroke={paint('gold-pale')} strokeWidth={1.25}/>
  </Accessory>,
  // Ink handlebar mustache resting on the mouth line, tips curled up.
  'mustache': () => <Accessory y="40-48">
    <path fill={paint('ink')} stroke={paint('gold-pale')} strokeWidth={1} strokeOpacity={0.65}
      d="M32 42 C35 40.5 40 40.5 43 42.5 C45 44 47.5 44 49.5 41.75 C50 45.25 47.5 47.25 43.5 47.25 C39.5 47.25 36 46 32 46 C28 46 24.5 47.25 20.5 47.25 C16.5 47.25 14 45.25 14.5 41.75 C16.5 44 19 44 21 42.5 C24 40.5 29 40.5 32 42 Z"/>
  </Accessory>,
  // Ink headband over the head, one ear cup with an oxblood cushion and a short mic.
  'headset': () => <Accessory y="6-40">
    {/* Band ends at y 24; its round cap paints to y 26 and tucks under the cup. */}
    <path d="M12.5 24 C11.5 15 20.5 9 32 9 C43.5 9 52.5 15 51.5 24" fill="none" stroke={paint('ink')} strokeWidth={4}
      strokeLinecap="round"/>
    <path d="M12.5 24 C11.5 15 20.5 9 32 9 C43.5 9 52.5 15 51.5 24" fill="none" stroke={paint('gold-pale')} strokeWidth={1}
      strokeOpacity={0.45}/>
    <path d="M14 36 C18 39 21 38.5 23 38" fill="none" stroke={paint('ink')} strokeWidth={1.75}/>
    <circle cx={23.5} cy={37.75} r={1.75} fill={paint('oxblood')} stroke={paint('ink')} strokeWidth={0.75}/>
    <rect x={8.5} y={24.5} width={8} height={12.5} rx={3.5} fill={paint('ink')} {...SHEEN}/>
    <rect x={14} y={26.5} width={3} height={8.5} rx={1.5} fill={paint('oxblood')}/>
  </Accessory>,
};
