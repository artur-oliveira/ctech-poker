import type {JSX, ReactNode, SVGProps} from 'react';

/*
 * Avatar frame art (DESIGN.md "Avatar cosmetics art contract"). Every frame is
 * a ring drawn in a 64x64 box over the avatar circle (centre 32,32, r=32): the
 * band lives between r≈25 and r=32 so the photo stays clear inside, and every
 * stroke ends at r<=32 so nothing leaves the box. Paint is only `--cosmetic-*`.
 * At a 24px seat one unit is 0.375px, so a stroke that has to read on its own
 * is >=4 units; thinner lines below are only contrast edges or shading.
 */

type Token = 'gold' | 'gold-pale' | 'oxblood' | 'felt' | 'walnut' | 'paper' | 'ink' | 'wine';

const paint = (token: Token) => `var(--cosmetic-${token})`;

/** A circle stroke centred on r, spanning r-w/2 .. r+w/2. */
function Ring({r, w, c, ...rest}: { r: number; w: number; c: Token } & SVGProps<SVGCircleElement>) {
  return <circle cx={32} cy={32} r={r} fill="none" stroke={paint(c)} strokeWidth={w} {...rest}/>;
}

/** A highlight arc over `sweep` degrees starting at `from` (0 = 3 o'clock, clockwise). */
function Arc({from, sweep, ...ring}: { from: number; sweep: number; r: number; w: number; c: Token; strokeOpacity?: number }) {
  return <Ring {...ring} pathLength={360} strokeDasharray={`${sweep} ${360 - sweep}`}
    strokeDashoffset={-from} strokeLinecap="round"/>;
}

function Frame({children}: { children: ReactNode }) {
  return <svg viewBox="0 0 64 64" aria-hidden="true" focusable="false">{children}</svg>;
}

export const FRAME_ART: Record<string, () => JSX.Element> = {
  // Two pale-gold rings split by an ink seam; at 24px it reads as a pale ring with a groove.
  'season-2026-q3': () => <Frame>
    <Ring r={25.25} w={0.75} c="ink"/>
    <Ring r={26.5} w={2} c="gold-pale"/>
    <Ring r={28} w={1} c="ink"/>
    <Ring r={29.85} w={2.7} c="gold-pale"/>
    <Ring r={31.6} w={0.8} c="ink"/>
  </Frame>,
  // Solid gold band with a pale highlight on the upper left, lit from above.
  'champion-gold': () => <Frame>
    <Ring r={25.5} w={1} c="ink"/>
    <Ring r={29} w={6} c="gold"/>
    <Arc r={29} w={2.5} c="gold-pale" from={160} sweep={140}/>
  </Frame>,
  // Casino chip edge: oxblood rim, 12 alternating segments, paper inlays inset from both edges.
  'chip-ring': () => <Frame>
    <Ring r={25.5} w={1} c="ink"/>
    <Ring r={29} w={6} c="oxblood"/>
    <Ring r={29} w={3.5} c="paper" pathLength={12} strokeDasharray="1 1" strokeDashoffset={0.5}/>
  </Frame>,
  // Table felt band with the walnut rail line on its inside edge.
  'felt-ring': () => <Frame>
    <Ring r={26} w={2} c="walnut"/>
    <Ring r={29.5} w={5} c="felt"/>
  </Frame>,
  // Walnut rail with a dark outer edge and a warm highlight arc.
  'walnut-rail': () => <Frame>
    <Ring r={25.25} w={0.5} c="ink"/>
    <Ring r={28.5} w={6} c="walnut"/>
    <Ring r={31.5} w={1} c="ink"/>
    <Arc r={29.5} w={2} c="gold" strokeOpacity={0.55} from={165} sweep={120}/>
  </Frame>,
  // Brand-red band with a thin gold line on the inside.
  'oxblood-edge': () => <Frame>
    <Ring r={25.75} w={1.5} c="gold"/>
    <Ring r={29.25} w={5.5} c="oxblood"/>
  </Frame>,
};
