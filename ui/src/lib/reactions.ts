export const TABLE_REACTIONS = {
  clap: {label: 'Aplausos', glyph: '👏', targeted: false},
  laugh: {label: 'Risada', glyph: '😄', targeted: false},
  wow: {label: 'Uau', glyph: '😮', targeted: false},
  angry: {label: 'Raiva', glyph: '😤', targeted: false},
  cry: {label: 'Choro', glyph: '😭', targeted: false},
  nervous: {label: 'Nervoso', glyph: '😰', targeted: false},
  cold: {label: 'Frio na mesa', glyph: '🥶', targeted: false},
  fire: {label: 'Sequência quente', glyph: '🔥', targeted: false},
  respect: {label: 'Respeito', glyph: '🫡', targeted: false},
  sleepy: {label: 'Sono', glyph: '🥱', targeted: false},

  chip: {label: 'Jogar ficha', glyph: '🟠', targeted: true},
  coffee: {label: 'Mandar café', glyph: '☕', targeted: true},
  clover: {label: 'Dar sorte', glyph: '🍀', targeted: true},
  horseshoe: {label: 'Jogar ferradura', glyph: '🧲', targeted: true},
  tear: {label: 'Jogar lágrima', glyph: '💧', targeted: true},
  tomato: {label: 'Jogar tomate', glyph: '🍅', targeted: true},
  poop: {label: 'Jogar cocô', glyph: '💩', targeted: true},
  rofl: {label: 'Rir da cara', glyph: '🤣', targeted: true},
  duck: {label: 'Jogar pato', glyph: '🦆', targeted: true},
  turtle: {label: 'Chamar de lento', glyph: '🐢', targeted: true}
} as const;

export type TableReactionID = keyof typeof TABLE_REACTIONS;

export function isTableReaction(value: string): value is TableReactionID {
  return value in TABLE_REACTIONS;
}

export interface TableReactionEvent {
  id: string;
  playerId: string;
  reactionId: TableReactionID;
  targetPlayerId?: string;
}
