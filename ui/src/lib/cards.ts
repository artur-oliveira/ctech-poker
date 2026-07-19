const ranks: Record<string, string> = {T: '10', J: 'jack', Q: 'queen', K: 'king', A: 'ace'};
const suits: Record<string, string> = {c: 'club', d: 'diamond', h: 'heart', s: 'spade'};

export function cardPath(c: string) {
  return `/svgs/${suits[c[1]]}-${ranks[c[0]] || c[0]}.svg`
}

export const back = '/svgs/card-back-red.svg'
