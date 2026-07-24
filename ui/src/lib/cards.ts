const ranks: Record<string, string> = {T: '10', J: 'jack', Q: 'queen', K: 'king', A: 'ace'};
const suits: Record<string, string> = {c: 'club', d: 'diamond', h: 'heart', s: 'spade'};
const rankLabels: Record<string, string> = {
  '2': 'dois', '3': 'três', '4': 'quatro', '5': 'cinco', '6': 'seis', '7': 'sete', '8': 'oito', '9': 'nove',
  T: 'dez', J: 'valete', Q: 'dama', K: 'rei', A: 'ás'
};
const suitLabels: Record<string, string> = {c: 'paus', d: 'ouros', h: 'copas', s: 'espadas'};

export function cardPath(c: string) {
  const normalized = c?.trim();
  const suit = suits[normalized?.[1]?.toLowerCase()], rankCode = normalized?.[0]?.toUpperCase();
  const rank = ranks[rankCode] || rankCode;
  return suit && rank ? `/svgs/${suit}-${rank}.svg` : back;
}

export function cardLabel(card: string) {
  const normalized = card?.trim();
  const rank = rankLabels[normalized?.[0]?.toUpperCase()], suit = suitLabels[normalized?.[1]?.toLowerCase()];
  return rank && suit ? `${rank} de ${suit}` : 'carta desconhecida';
}

export const back = '/svgs/card-back-red.svg';
