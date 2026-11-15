import {useId, type JSX, type ReactNode, type SVGProps} from 'react';

/*
 * Avatar frame art (DESIGN.md "Avatar cosmetics art contract"). Every frame is
 * a ring drawn in a 64x64 box over the avatar circle (centre 32,32, r=32): the
 * band lives between r≈25 and r=32 so the photo stays clear inside, and every
 * stroke ends at r<=32 so nothing leaves the box. Paint is only `--cosmetic-*`
 * (directly, or through a gradient whose stops are tokens).
 * At a 24px seat one unit is 0.375px, so a stroke that has to read on its own
 * is >=4 units; thinner lines below are only contrast edges or shading.
 * The grammar is a calm main band plus hairlines: an ink edge on the photo
 * side, engraved rules, one specular sweep lit from the upper left.
 *
 * Animated frames are a static base svg plus `Fx` layers: whole <svg>
 * elements that renderer.css moves by `data-fx` with transform/opacity only,
 * so every frame of the animation is a compositor job (no paint, no layout).
 * Each layer's resting pose is part of the static frame that reduced motion
 * and inactive seats show, so it is drawn to look finished at rest. Fx
 * layers may reach past r=32 (flames, aura glow, twinkles): overflow visible.
 */

type Token =
  | 'gold' | 'gold-pale' | 'gold-deep' | 'oxblood' | 'oxblood-bright' | 'felt' | 'walnut' | 'walnut-deep'
  | 'paper' | 'ink' | 'wine' | 'pearl-shade' | 'silver' | 'silver-pale' | 'silver-deep'
  | 'rose' | 'rose-pale' | 'rose-deep' | 'obsidian' | 'obsidian-lift' | 'emerald' | 'emerald-pale' | 'emerald-deep'
  | 'sapphire' | 'sapphire-pale' | 'sapphire-deep' | 'olive' | 'olive-deep'
  | 'ember' | 'flame' | 'amethyst' | 'amethyst-pale' | 'amethyst-deep' | 'spark';
type Paint = Token | `url(#${string})`;

const paint = (p: Paint) => (p.startsWith('url(') ? p : `var(--cosmetic-${p})`);

/** A circle stroke centred on r, spanning r-w/2 .. r+w/2. */
function Ring({r, w, c, ...rest}: { r: number; w: number; c: Paint } & SVGProps<SVGCircleElement>) {
  return <circle cx={32} cy={32} r={r} fill="none" stroke={paint(c)} strokeWidth={w} {...rest}/>;
}

/** An arc over `sweep` degrees starting at `from` (0 = 3 o'clock, clockwise; upper left is 180-270). */
function Arc({from, sweep, ...ring}: { from: number; sweep: number; r: number; w: number; c: Paint; strokeOpacity?: number }) {
  return <Ring {...ring} pathLength={360} strokeDasharray={`${sweep} ${360 - sweep}`}
    strokeDashoffset={-from} strokeLinecap="round"/>;
}

/** `n` round beads of diameter w on a circle of radius r (a zero-length dash with round caps). */
function Beads({n, offset = 0, ...ring}: { n: number; offset?: number; r: number; w: number; c: Paint; strokeOpacity?: number }) {
  return <Ring {...ring} pathLength={n} strokeDasharray="0 1" strokeDashoffset={offset} strokeLinecap="round"/>;
}

/** Points on the ring at `deg` (0 = 3 o'clock, clockwise), for studs and inlays. */
const at = (deg: number, r: number) => {
  const a = (deg * Math.PI) / 180;
  return {cx: 32 + r * Math.cos(a), cy: 32 + r * Math.sin(a)};
};

/**
 * A diagonal metal gradient, light upper left to deep lower right. The id is
 * per instance: a frame renders many times on one page (seats, leaderboard),
 * and a repeated id breaks `url(#id)` as soon as the first copy is hidden.
 */
function useMetal(light: Token, base: Token, deep: Token): [ReactNode, Paint] {
  const id = `frame-metal-${useId().replace(/[^\w-]/g, '')}`;
  return [<linearGradient key={id} id={id} gradientUnits="userSpaceOnUse" x1={8} y1={8} x2={56} y2={56}>
    <stop offset="0" stopColor={paint(light)}/>
    <stop offset="0.45" stopColor={paint(base)}/>
    <stop offset="1" stopColor={paint(deep)}/>
  </linearGradient>, `url(#${id})`];
}

function Frame({defs, children}: { defs?: ReactNode; children: ReactNode }) {
  return <svg viewBox="0 0 64 64" aria-hidden="true" focusable="false">
    {defs && <defs>{defs}</defs>}
    {children}
  </svg>;
}

const COMPASS = [270, 0, 90, 180];

// Pale-gold double rule: a 4-unit band and an outer hairline, a fine row of beads on the photo side.
function SeasonFrame() {
  const [g, metal] = useMetal('paper', 'gold-pale', 'gold');
  return <Frame defs={g}>
    <Ring r={25} w={0.8} c="ink"/>
    <Beads n={44} r={26.2} w={1.5} c="gold-pale"/>
    <Ring r={27.25} w={0.5} c="ink"/>
    <Ring r={29.5} w={4} c={metal}/>
    <Ring r={31.75} w={0.5} c="gold-deep"/>
  </Frame>;
}

// Polished gold: wide gradient band, two engraved hairlines, one sweep, studs at 12/3/6/9.
function ChampionFrame() {
  const [g, metal] = useMetal('gold-pale', 'gold', 'gold-deep');
  return <Frame defs={g}>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={28.75} w={6.5} c={metal}/>
    <Ring r={26.3} w={0.45} c="gold-deep"/>
    <Ring r={31.2} w={0.45} c="gold-deep"/>
    <Arc r={28.75} w={1.6} c="gold-pale" strokeOpacity={0.8} from={192} sweep={62}/>
    {COMPASS.map(d => <circle key={d} {...at(d, 28.75)} r={1.35} fill={paint('gold-pale')}
      stroke={paint('gold-deep')} strokeWidth={0.45}/>)}
  </Frame>;
}

// Casino chip edge: oxblood rim, 12 alternating segments, crisp paper inlays inset from both edges.
function ChipFrame() {
  const [g, rim] = useMetal('oxblood-bright', 'oxblood', 'wine');
  return <Frame defs={g}>
    <Ring r={25.25} w={0.8} c="ink"/>
    <Ring r={28.85} w={6.3} c={rim}/>
    <Ring r={28.85} w={3.6} c="paper" pathLength={12} strokeDasharray="0.9 1.1" strokeDashoffset={0.45}/>
    <Ring r={26.35} w={0.4} c="paper" strokeOpacity={0.55}/>
    <Ring r={31.65} w={0.6} c="wine"/>
  </Frame>;
}

// Table felt with a stitched seam and the walnut rail rule on the photo side.
function FeltFrame() {
  return <Frame>
    <Ring r={24.85} w={0.7} c="ink"/>
    <Ring r={26} w={1.8} c="walnut"/>
    <Ring r={29.45} w={5.1} c="felt"/>
    <Ring r={29.3} w={0.6} c="paper" strokeOpacity={0.5} pathLength={72} strokeDasharray="0.55 0.45"/>
    <Ring r={31.6} w={0.8} c="ink" strokeOpacity={0.35}/>
  </Frame>;
}

// Walnut rail: brass hairline inside, grain as offset darker/lighter arcs, a warm highlight.
function WalnutFrame() {
  const [g, wood] = useMetal('walnut', 'walnut', 'walnut-deep');
  return <Frame defs={g}>
    <Ring r={25.05} w={0.6} c="ink"/>
    <Ring r={25.85} w={0.9} c="gold"/>
    <Ring r={29.1} w={5.8} c={wood}/>
    <Arc r={27.6} w={0.5} c="walnut-deep" strokeOpacity={0.8} from={15} sweep={120}/>
    <Arc r={30.4} w={0.45} c="walnut-deep" strokeOpacity={0.7} from={150} sweep={140}/>
    <Arc r={28.9} w={0.4} c="gold" strokeOpacity={0.3} from={300} sweep={80}/>
    <Arc r={29.4} w={1.6} c="gold" strokeOpacity={0.45} from={190} sweep={75}/>
    <Ring r={31.7} w={0.6} c="ink"/>
  </Frame>;
}

// Oxblood enamel: gold inner hairline, a deep red band, one pale gloss arc.
function OxbloodFrame() {
  const [g, enamel] = useMetal('oxblood-bright', 'oxblood', 'wine');
  return <Frame defs={g}>
    <Ring r={24.9} w={0.6} c="ink"/>
    <Ring r={25.75} w={1.1} c="gold"/>
    <Ring r={29.1} w={5.6} c={enamel}/>
    <Arc r={29.6} w={1.3} c="paper" strokeOpacity={0.35} from={198} sweep={55}/>
    <Ring r={31.7} w={0.6} c="wine"/>
  </Frame>;
}

// Silver band with an engraved rule and a filigree rhythm of large/small beads on the outer edge.
function SilverFrame() {
  const [g, metal] = useMetal('silver-pale', 'silver', 'silver-deep');
  return <Frame defs={g}>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={28.2} w={5} c={metal}/>
    <Ring r={26.4} w={0.4} c="silver-deep"/>
    <Arc r={28.2} w={1.3} c="silver-pale" strokeOpacity={0.7} from={195} sweep={55}/>
    <Ring r={31.3} w={1.4} c="silver-deep"/>
    <Beads n={20} r={30.95} w={1.6} c="silver-pale"/>
    <Beads n={20} offset={0.5} r={31.2} w={0.8} c="silver-deep"/>
  </Frame>;
}

// Sixteen pearls on a wine thread, each lit from its own upper left.
function PearlFrame() {
  const id = `frame-pearl-${useId().replace(/[^\w-]/g, '')}`;
  return <Frame defs={<radialGradient id={id} cx={0.36} cy={0.32} r={0.75}>
    <stop offset="0" stopColor={paint('paper')}/>
    <stop offset="0.55" stopColor={paint('paper')}/>
    <stop offset="1" stopColor={paint('pearl-shade')}/>
  </radialGradient>}>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={28.6} w={2.2} c="wine"/>
    {Array.from({length: 16}, (_, i) => <circle key={i} {...at(i * 22.5, 28.6)} r={3}
      fill={`url(#${id})`} stroke={paint('pearl-shade')} strokeWidth={0.3}/>)}
  </Frame>;
}

// Rose metal: a calm gradient band and a double hairline, nothing else.
function RoseFrame() {
  const [g, metal] = useMetal('rose-pale', 'rose', 'rose-deep');
  return <Frame defs={g}>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={28.4} w={5.4} c={metal}/>
    <Ring r={26.5} w={0.4} c="rose-pale" strokeOpacity={0.8}/>
    <Arc r={28.4} w={1.4} c="rose-pale" strokeOpacity={0.6} from={195} sweep={60}/>
    <Ring r={31.65} w={0.7} c="rose"/>
  </Frame>;
}

// Near-black glossy band, one thin gold rule on the photo side, a cold specular arc.
function ObsidianFrame() {
  const [g, glass] = useMetal('obsidian-lift', 'obsidian', 'ink');
  return <Frame defs={g}>
    <Ring r={24.9} w={0.6} c="ink"/>
    <Ring r={25.75} w={0.9} c="gold"/>
    <Ring r={29} w={6} c={glass}/>
    <Arc r={29.4} w={1.4} c="silver-pale" strokeOpacity={0.6} from={196} sweep={48}/>
    <Ring r={31.75} w={0.5} c="silver-deep"/>
  </Frame>;
}

// Emerald cut: eight facets, each split crown/pavilion so light and dark alternate like a cut stone.
function EmeraldFrame() {
  return <Frame>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={27.1} w={3} c="emerald"/>
    <Ring r={27.1} w={3} c="emerald-pale" pathLength={8} strokeDasharray="1 1"/>
    <Ring r={30.1} w={3} c="emerald-deep"/>
    <Ring r={30.1} w={3} c="emerald" pathLength={8} strokeDasharray="1 1"/>
    {Array.from({length: 8}, (_, i) => {
      const a = at(i * 45, 25.6);
      const b = at(i * 45, 31.6);
      return <line key={i} x1={a.cx} y1={a.cy} x2={b.cx} y2={b.cy} stroke={paint('ink')} strokeOpacity={0.45} strokeWidth={0.35}/>;
    })}
    <Ring r={31.75} w={0.5} c="gold-pale"/>
  </Frame>;
}

// Art déco sapphire: gold rules either side and stepped gold notches at the eight compass points.
const DECO_STEP = 'M29 0.9H35V2.2H34V3.6H33V5.2H31V3.6H30V2.2H29Z';
function SapphireFrame() {
  const [g, enamel] = useMetal('sapphire-pale', 'sapphire', 'sapphire-deep');
  return <Frame defs={g}>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={25.95} w={0.8} c="gold"/>
    <Ring r={29} w={5.6} c={enamel}/>
    <Ring r={31.65} w={0.7} c="gold"/>
    {Array.from({length: 8}, (_, i) => <path key={i} d={DECO_STEP} fill={paint('gold')}
      transform={`rotate(${i * 45} 32 32)`}/>)}
  </Frame>;
}

// Two mirrored olive branches on a dark olive band, growing from the bottom and meeting at the top.
const LEAF_ANGLES = [118, 142, 166, 190, 214, 238];
function OliveBranch() {
  return <g>
    {LEAF_ANGLES.map((a, i) => <g key={a} transform={`rotate(${a - 270} 32 32)`}>
      <ellipse cx={33.5} cy={2.3} rx={2.5} ry={1.05} transform="rotate(-28 33.5 2.3)"
        fill={paint(i % 2 ? 'gold' : 'olive')}/>
      <ellipse cx={33.5} cy={5.2} rx={2.5} ry={1.05} transform="rotate(28 33.5 5.2)"
        fill={paint(i % 2 ? 'olive' : 'gold')}/>
    </g>)}
  </g>;
}
function OliveFrame() {
  return <Frame>
    <Ring r={25.2} w={0.7} c="ink"/>
    <Ring r={28.65} w={6.3} c="olive-deep"/>
    <Arc r={28.3} w={0.8} c="gold-pale" from={104} sweep={332}/>
    <OliveBranch/>
    <g transform="translate(64 0) scale(-1 1)"><OliveBranch/></g>
    <circle cx={32} cy={3.7} r={1.4} fill={paint('gold')} stroke={paint('gold-deep')} strokeWidth={0.4}/>
  </Frame>;
}

// Gold ring with the four suits as paper inlays at N/E/S/W.
const SUIT_PIPS: { deg: number; c: Token; d: string }[] = [
  {deg: 270, c: 'ink', d: 'M0-2.3C-2.6-.2-2.4 1.6-1.2 1.6-.5 1.6 0 1 0 .6 0 1 .5 1.6 1.2 1.6 2.4 1.6 2.6-.2 0-2.3ZM-.65 2.3L0 .6.65 2.3Z'},
  {deg: 0, c: 'oxblood', d: 'M0 2.2C-2.6.2-2.4-2.2-1.2-2.2-.5-2.2 0-1.6 0-1.1 0-1.6.5-2.2 1.2-2.2 2.4-2.2 2.6.2 0 2.2Z'},
  {deg: 90, c: 'oxblood', d: 'M0-2.4L1.7 0 0 2.4-1.7 0Z'},
  {deg: 180, c: 'ink', d: 'M-1-1.1a1 1 0 1 0 2 0a1 1 0 1 0-2 0ZM-2.1.4a1 1 0 1 0 2 0a1 1 0 1 0-2 0ZM.1.4a1 1 0 1 0 2 0a1 1 0 1 0-2 0ZM-.65 2.2L0 .2.65 2.2Z'},
];
function SuitFrame() {
  const [g, metal] = useMetal('gold-pale', 'gold', 'gold-deep');
  return <Frame defs={g}>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={28.75} w={6.2} c={metal}/>
    <Ring r={26.1} w={0.4} c="gold-deep"/>
    <Ring r={31.4} w={0.4} c="gold-deep"/>
    {SUIT_PIPS.map(({deg, c, d}) => {
      const {cx, cy} = at(deg, 28.75);
      return <g key={deg} transform={`translate(${cx} ${cy})`}>
        <circle r={2.95} fill={paint('paper')} stroke={paint('gold-deep')} strokeWidth={0.5}/>
        <path d={d} fill={paint(c)} transform="scale(0.92)"/>
      </g>;
    })}
  </Frame>;
}


const f2 = (n: number) => n.toFixed(2);
/** [x, y] at radius r and angle deg (0 = 3 o'clock, clockwise). */
const polar = (r: number, deg: number): [number, number] => {
  const a = (deg * Math.PI) / 180;
  return [32 + r * Math.cos(a), 32 + r * Math.sin(a)];
};
const pt = ([x, y]: [number, number]) => `${f2(x)} ${f2(y)}`;
/** A closed polyline through r(deg) sampled every 2 degrees, from `from` over `sweep`. */
const wave = (r: (deg: number) => number, from = 0, sweep = 360) => {
  const n = Math.round(sweep / 2);
  return Array.from({length: n + 1}, (_, i) => {
    const d = from + (sweep * i) / n;
    return `${i ? 'L' : 'M'}${pt(polar(r(d), d))}`;
  }).join(' ') + (sweep === 360 ? ' Z' : '');
};

/** One animated layer: the whole <svg> is what renderer.css moves (`data-fx`). */
function Fx({fx, children}: { fx: string; children: ReactNode }) {
  return <svg viewBox="0 0 64 64" aria-hidden="true" focusable="false" overflow="visible" data-fx={fx}>{children}</svg>;
}

/** A flame tongue rooted on r0 between deg±half, licking `len` outward and bending `bend` degrees. */
const tongue = (deg: number, half: number, r0: number, len: number, bend: number) =>
  `M${pt(polar(r0, deg - half))} Q${pt(polar(r0 + len * 0.6, deg - half * 0.7 + bend * 0.3))} ${pt(polar(r0 + len, deg + bend))}`
  + ` Q${pt(polar(r0 + len * 0.45, deg + half * 0.8 + bend * 0.2))} ${pt(polar(r0, deg + half))} Z`;
/** Ring of `n` tongues, taller at the top and bent upward like fire rising off a ring. */
const flames = (n: number, offset: number, r0: number, short: number, tall: number) =>
  Array.from({length: n}, (_, i) => {
    const deg = offset + (360 * i) / n;
    const up = (1 - Math.sin((deg * Math.PI) / 180)) / 2; // 1 at the top, 0 at the bottom
    return tongue(deg, 180 / n, r0, short + (tall - short) * up, -9 * Math.cos((deg * Math.PI) / 180));
  }).join(' ');

// Inferno: an oxblood band in a crown of flames, taller at the top; the outer
// tongues lick upward and the hot inner ones flicker (animated layers).
function InfernoFrame() {
  const [g, rim] = useMetal('oxblood-bright', 'oxblood', 'wine');
  return <>
    <Fx fx="flame">
      <path d={flames(13, 0, 27, 5.5, 10.5)} fill={paint('oxblood-bright')} stroke={paint('wine')} strokeWidth={0.7}/>
      <path d={flames(13, 0, 27, 3.8, 7.6)} fill={paint('ember')}/>
    </Fx>
    <Frame defs={g}>
      <Ring r={25.1} w={0.8} c="ink"/>
      <Ring r={28.6} w={5.8} c={rim}/>
      <Ring r={26.4} w={0.6} c="ember" strokeOpacity={0.8}/>
      <Ring r={31.3} w={0.6} c="wine"/>
    </Frame>
    <Fx fx="flame-core">
      <path d={flames(13, 180 / 13, 29.5, 2.4, 5)} fill={paint('ember')} stroke={paint('oxblood-bright')} strokeWidth={0.5}/>
      <path d={flames(13, 180 / 13, 30.2, 1.2, 3)} fill={paint('flame')}/>
    </Fx>
  </>;
}

// Arcane aura: an amethyst band breathing a soft violet glow (stacked
// translucent strokes, never a blur filter) with light motes orbiting it.
function ArcaneFrame() {
  const [g, band] = useMetal('amethyst-pale', 'amethyst', 'amethyst-deep');
  const mote = (deg: number, r: number, size: number) => {
    const [cx, cy] = polar(r, deg);
    return <g key={deg}>
      <circle cx={cx} cy={cy} r={size * 2} fill={paint('amethyst-pale')} fillOpacity={0.35}/>
      <circle cx={cx} cy={cy} r={size} fill={paint('paper')}/>
    </g>;
  };
  return <>
    <Fx fx="breathe">
      <Ring r={31.5} w={7} c="amethyst" strokeOpacity={0.16}/>
      <Ring r={32.5} w={4} c="amethyst-pale" strokeOpacity={0.22}/>
    </Fx>
    <Frame defs={g}>
      <Ring r={25.1} w={0.8} c="ink"/>
      <Ring r={28.6} w={5.6} c={band}/>
      <Ring r={26.3} w={0.45} c="amethyst-pale" strokeOpacity={0.7}/>
      <Ring r={31.2} w={0.6} c="amethyst-deep"/>
      <Arc r={28.6} w={1.3} c="amethyst-pale" strokeOpacity={0.6} from={196} sweep={50}/>
    </Frame>
    <Fx fx="orbit">{[220, 340, 100].map(d => mote(d, 28.6, 1.35))}</Fx>
    <Fx fx="orbit-back">{[40, 160, 280].map(d => mote(d, 31.6, 0.8))}</Fx>
  </>;
}

/** A zigzag bolt along the ring from `from` over `sweep` degrees. */
const bolt = (from: number, sweep: number, kinks: number) => Array.from({length: kinks + 1}, (_, i) =>
  `${i ? 'L' : 'M'}${pt(polar(i % 2 ? 30.6 : 26.8, from + (sweep * i) / kinks))}`).join(' ');

// Storm ring: a cold sapphire band; electric arcs run around it and flicker.
function StormFrame() {
  const [g, band] = useMetal('sapphire-pale', 'sapphire', 'sapphire-deep');
  const arc = (from: number, sweep: number, kinks: number) => <>
    <path d={bolt(from, sweep, kinks)} fill="none" stroke={paint('sapphire-pale')} strokeOpacity={0.5} strokeWidth={3.2}
      strokeLinejoin="round" strokeLinecap="round"/>
    <path d={bolt(from, sweep, kinks)} fill="none" stroke={paint('spark')} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round"/>
  </>;
  return <>
    <Frame defs={g}>
      <Ring r={25.1} w={0.8} c="ink"/>
      <Ring r={28.7} w={6.2} c="sapphire-deep"/>
      <Ring r={27.1} w={1.6} c={band}/>
      <Ring r={30.6} w={1} c={band}/>
      <Ring r={31.7} w={0.6} c="silver-pale"/>
    </Frame>
    <Fx fx="arc">{arc(188, 74, 7)}</Fx>
    <Fx fx="arc-back">{arc(18, 46, 5)}</Fx>
  </>;
}

/** A four-point twinkle of radius s at (cx, cy). */
const star = ([cx, cy]: [number, number], s: number) => {
  const k = s * 0.22;
  return `M${f2(cx)} ${f2(cy - s)} L${f2(cx + k)} ${f2(cy - k)} L${f2(cx + s)} ${f2(cy)} L${f2(cx + k)} ${f2(cy + k)}`
    + ` L${f2(cx)} ${f2(cy + s)} L${f2(cx - k)} ${f2(cy + k)} L${f2(cx - s)} ${f2(cy)} L${f2(cx - k)} ${f2(cy - k)} Z`;
};

// Gold shimmer: polished gold with a pale bevel; a specular sweep travels
// around it and two or three twinkles catch the light.
function ShimmerFrame() {
  const [g, metal] = useMetal('gold-pale', 'gold', 'gold-deep');
  return <>
    <Frame defs={g}>
      <Ring r={25.1} w={0.8} c="ink"/>
      <Ring r={28.75} w={6.5} c={metal}/>
      <Ring r={26.5} w={1.8} c="gold-pale"/>
      <Ring r={31.35} w={0.5} c="gold-deep"/>
    </Frame>
    <Fx fx="sweep">
      <Arc r={28.75} w={5} c="gold-pale" strokeOpacity={0.5} from={184} sweep={70}/>
      <Arc r={28.75} w={2.8} c="paper" strokeOpacity={0.9} from={204} sweep={30}/>
    </Fx>
    <Fx fx="twinkle">
      <path d={star(polar(30.5, 318), 5)} fill={paint('paper')} stroke={paint('gold-deep')} strokeWidth={0.4}/>
      <path d={star(polar(30.5, 128), 4)} fill={paint('paper')} stroke={paint('gold-deep')} strokeWidth={0.4}/>
    </Fx>
    <Fx fx="twinkle-late">
      <path d={star(polar(30.5, 48), 4.2)} fill={paint('paper')} stroke={paint('gold-deep')} strokeWidth={0.4}/>
    </Fx>
  </>;
}

// Baroque gilt: a gilt band with a scrolled cartouche at 12/3/6/9, each a pale
// plaque between two curled volutes with an oxblood cabochon.
function BaroqueFrame() {
  const [g, metal] = useMetal('gold-pale', 'gold', 'gold-deep');
  const volute = 'M27.2 3.2 C26.6 0.6 23.2 0.4 22.6 2.6 C22.1 4.6 24.4 5.9 25.6 4.6 C24.6 4.4 24 3.4 24.8 2.6 C25.6 1.9 26.8 2.4 27.2 3.2 Z';
  return <Frame defs={g}>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={28.7} w={5.4} c={metal}/>
    <Ring r={26.3} w={0.45} c="gold-deep"/>
    <Ring r={31.15} w={0.45} c="gold-deep"/>
    {COMPASS.map(d => <g key={d} transform={`rotate(${d + 90} 32 32)`}>
      <path d={volute} fill={paint('gold-deep')} stroke={paint('gold-pale')} strokeWidth={0.4}/>
      <path d={volute} transform="translate(64 0) scale(-1 1)" fill={paint('gold-deep')} stroke={paint('gold-pale')} strokeWidth={0.4}/>
      <ellipse cx={32} cy={3.3} rx={4.3} ry={2.9} fill={paint('gold-pale')} stroke={paint('gold-deep')} strokeWidth={0.6}/>
      <circle cx={32} cy={3.3} r={1.35} fill={paint('oxblood')} stroke={paint('wine')} strokeWidth={0.35}/>
    </g>)}
  </Frame>;
}

/** Laurel leaves along r=28.6, rooted every `step` degrees from `from` toward `to`. */
function laurelLeaves(from: number, to: number, step: number) {
  const out: { d: string; pale: boolean }[] = [];
  const dir = Math.sign(to - from);
  for (let deg = from, i = 0; dir * (to - deg) >= 0; deg += dir * step, i++) {
    const [bx, by] = polar(28.6, deg);
    const tangent = ((deg + 90 * dir) * Math.PI) / 180;
    for (const side of [-1, 1]) {
      const a = tangent + side * 0.62 * dir;
      const tx = bx + 5.2 * Math.cos(a), ty = by + 5.2 * Math.sin(a);
      const mx = (bx + tx) / 2, my = (by + ty) / 2, nx = -Math.sin(a) * 1.9, ny = Math.cos(a) * 1.9;
      out.push({pale: (i + (side > 0 ? 1 : 0)) % 2 === 0,
        d: `M${f2(bx)} ${f2(by)} Q${f2(mx + nx)} ${f2(my + ny)} ${f2(tx)} ${f2(ty)} Q${f2(mx - nx)} ${f2(my - ny)} ${f2(bx)} ${f2(by)} Z`});
    }
  }
  return out;
}

// Laurel wreath: no band, a full ring of gold leaves on two branches rising from
// an oxblood ribbon at the bottom and meeting at the top.
function LaurelWreathFrame() {
  const leaves = [...laurelLeaves(100, 262, 13.5), ...laurelLeaves(80, -82, 13.5)];
  return <Frame>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={28.6} w={1.6} c="gold-deep"/>
    {leaves.map(({d, pale}, i) => <path key={i} d={d} fill={paint(pale ? 'gold-pale' : 'gold')}
      stroke={paint('gold-deep')} strokeWidth={0.5}/>)}
    <path d="M32 59.2 L27.2 56.4 L27.6 62 Z M32 59.2 L36.8 56.4 L36.4 62 Z" fill={paint('oxblood')} stroke={paint('wine')}
      strokeWidth={0.5} strokeLinejoin="round"/>
    <circle cx={32} cy={59.2} r={1.5} fill={paint('oxblood-bright')} stroke={paint('wine')} strokeWidth={0.4}/>
  </Frame>;
}

// Art nouveau: a whiplash vine wanders across a deep emerald band between gold
// rules, a gold leaf on each outer crest and a pale bud in each hollow.
function NouveauFrame() {
  const crest = (deg: number) => 28.6 + 1.9 * Math.sin((6 * deg * Math.PI) / 180);
  return <Frame>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={25.8} w={0.8} c="gold"/>
    <Ring r={28.8} w={5.4} c="emerald-deep"/>
    <Ring r={31.65} w={0.7} c="gold"/>
    <path d={wave(crest)} fill="none" stroke={paint('emerald-pale')} strokeWidth={1.3} strokeLinejoin="round"/>
    {Array.from({length: 6}, (_, i) => {
      const crestDeg = 15 + i * 60, hollow = 45 + i * 60;
      const [lx, ly] = polar(30.2, crestDeg);
      return <g key={i}>
        <ellipse cx={lx} cy={ly} rx={2.4} ry={1.15} transform={`rotate(${crestDeg + 60} ${f2(lx)} ${f2(ly)})`}
          fill={paint('gold')} stroke={paint('gold-deep')} strokeWidth={0.4}/>
        <circle {...at(hollow, 26.9)} r={1.05} fill={paint('gold-pale')}/>
      </g>;
    })}
  </Frame>;
}

// Celtic knot: two silver strands woven over and under each other around an
// emerald band, every crossing alternating which strand is on top.
function CelticFrame() {
  const strand = (sign: number) => (deg: number) => 28.7 + sign * 2.1 * Math.sin((8 * deg * Math.PI) / 180);
  const cord = (d: string, key?: string) => <g key={key}>
    <path d={d} fill="none" stroke={paint('ink')} strokeWidth={2.7} strokeLinejoin="round"/>
    <path d={d} fill="none" stroke={paint('silver-pale')} strokeWidth={1.5} strokeLinejoin="round"/>
  </g>;
  return <Frame>
    <Ring r={25.1} w={0.8} c="ink"/>
    <Ring r={28.7} w={6.4} c="emerald-deep"/>
    <Ring r={25.9} w={0.5} c="emerald" />
    <Ring r={31.6} w={0.6} c="silver"/>
    {cord(wave(strand(1)))}
    {cord(wave(strand(-1)))}
    {/* Crossings sit every 22.5 degrees; every other one is redrawn with the first strand on top. */}
    {Array.from({length: 8}, (_, i) => cord(wave(strand(1), i * 45 - 7, 14), String(i)))}
  </Frame>;
}

export const FRAME_ART: Record<string, () => JSX.Element> = {
  'season-2026-q3': SeasonFrame,
  'champion-gold': ChampionFrame,
  'chip-ring': ChipFrame,
  'felt-ring': FeltFrame,
  'walnut-rail': WalnutFrame,
  'oxblood-edge': OxbloodFrame,
  'silver-filigree': SilverFrame,
  'pearl-beads': PearlFrame,
  'rose-gold': RoseFrame,
  'obsidian-edge': ObsidianFrame,
  'emerald-cut': EmeraldFrame,
  'sapphire-deco': SapphireFrame,
  'olive-branch': OliveFrame,
  'suit-ring': SuitFrame,
  'inferno': InfernoFrame,
  'arcane-aura': ArcaneFrame,
  'storm-ring': StormFrame,
  'gold-shimmer': ShimmerFrame,
  'baroque-gilt': BaroqueFrame,
  'laurel-wreath': LaurelWreathFrame,
  'art-nouveau': NouveauFrame,
  'celtic-knot': CelticFrame,
};
