export type SoundName = 'reveal' | 'showing_card' | 'half_pot' | 'all_in' | 'bet' | 'your_turn';

const FILES: Record<SoundName, string[]> = {
  reveal: ['/sounds/revealing-card-table.mp3'],
  showing_card: ['/sounds/player-showing-card.mp3'],
  half_pot: ['/sounds/half-pot-chips.mp3'],
  all_in: ['/sounds/all-in-chips.mp3'],
  bet: ['/sounds/basic-chips-1.mp3', '/sounds/basic-chips-2.mp3'],
  your_turn: ['/sounds/your-turn.mp3']
};

// Module-scoped to match the table token/preferences lifetime without making
// every realtime transition depend on a React render. Starts false so sound
// is never emitted before the player's persisted opt-in has been read.
let effectsEnabled = false;

export function setSoundEffectsEnabled(enabled: boolean) {
  effectsEnabled = enabled;
}

export function soundEffectsEnabled() {
  return effectsEnabled;
}

// .catch swallows the common autoplay-blocked-before-user-interaction
// rejection: not a real application error.
export function playSound(name: SoundName) {
  if (!effectsEnabled) return false;
  const files = FILES[name];
  const file = files[Math.floor(Math.random() * files.length)];
  new Audio(file).play().catch(() => {
  });
  return true;
}
