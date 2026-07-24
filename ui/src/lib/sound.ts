export type SoundName = 'reveal' | 'showing_card' | 'half_pot' | 'all_in' | 'bet';

const FILES: Record<SoundName, string[]> = {
  reveal: ['/sounds/revealing-card-table.mp3'],
  showing_card: ['/sounds/player-showing-card.mp3'],
  half_pot: ['/sounds/half-pot-chips.mp3'],
  all_in: ['/sounds/all-in-chips.mp3'],
  bet: ['/sounds/basic-chips-1.mp3', '/sounds/basic-chips-2.mp3']
};

// .catch swallows the common autoplay-blocked-before-user-interaction
// rejection — not a real application error.
export function playSound(name: SoundName) {
  const files = FILES[name];
  const file = files[Math.floor(Math.random() * files.length)];
  new Audio(file).play().catch(() => {
  });
}
