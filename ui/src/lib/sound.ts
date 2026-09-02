export type SoundName = 'reveal' | 'showing_card' | 'half_pot' | 'all_in' | 'bet' | 'your_turn';

const FILES: Record<SoundName, string[]> = {
  reveal: ['/sounds/revealing-card-table.mp3'],
  showing_card: ['/sounds/player-showing-card.mp3'],
  half_pot: ['/sounds/half-pot-chips.mp3'],
  all_in: ['/sounds/all-in-chips.mp3'],
  bet: ['/sounds/basic-chips-1.mp3', '/sounds/basic-chips-2.mp3'],
  your_turn: ['/sounds/your-turn.mp3']
};

const ALL_FILES = [...new Set(Object.values(FILES).flat())];

// Module-scoped to match the table token/preferences lifetime without making
// every realtime transition depend on a React render. Starts false so sound
// is never emitted before the player's persisted opt-in has been read.
let effectsEnabled = false;

// One pooled, preloaded HTMLAudioElement per sound file. Populated on opt-in so
// the first play of each cue does not wait on its own fetch, and released on
// opt-out so we hold no audio buffers while the player has sound off.
const pool = new Map<string, HTMLAudioElement>();

function releasePool() {
  for (const element of pool.values()) {
    element.pause();
    element.removeAttribute('src');
    element.load();
  }
  pool.clear();
}

function primePool() {
  if (typeof Audio === 'undefined') return;
  for (const file of ALL_FILES) {
    const element = new Audio(file);
    element.preload = 'auto';
    element.load();
    // One play/pause inside the enabling user gesture unlocks the element for
    // later programmatic playback under browser autoplay policies.
    element.play().then(() => {
      element.pause();
      element.currentTime = 0;
    }).catch(() => {
    });
    pool.set(file, element);
  }
}

export function setSoundEffectsEnabled(enabled: boolean) {
  if (enabled === effectsEnabled) return;
  effectsEnabled = enabled;
  if (enabled) primePool();
  else releasePool();
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
  const pooled = pool.get(file);
  const element = pooled ?? new Audio(file);
  element.currentTime = 0;
  element.play().catch(() => {
  });
  return true;
}
