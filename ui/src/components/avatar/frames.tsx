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
 */

type Token =
  | 'gold' | 'gold-pale' | 'gold-deep' | 'oxblood' | 'oxblood-bright' | 'felt' | 'walnut' | 'walnut-deep'
  | 'paper' | 'ink' | 'wine' | 'pearl-shade' | 'silver' | 'silver-pale' | 'silver-deep'
  | 'rose' | 'rose-pale' | 'rose-deep' | 'obsidian' | 'obsidian-lift' | 'emerald' | 'emerald-pale' | 'emerald-deep'
  | 'sapphire' | 'sapphire-pale' | 'sapphire-deep' | 'olive' | 'olive-deep';
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
};
