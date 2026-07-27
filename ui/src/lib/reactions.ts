export const TABLE_REACTIONS = {
  clap: {label: 'Aplausos', glyph: '👏', targeted: false},
  laugh: {label: 'Risada', glyph: '😄', targeted: false},
  wow: {label: 'Uau', glyph: '😮', targeted: false},
  chip: {label: 'Jogar ficha', glyph: '🟠', targeted: true},
  coffee: {label: 'Mandar café', glyph: '☕', targeted: true},
  clover: {label: 'Dar sorte', glyph: '🍀', targeted: true}
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
