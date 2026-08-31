export const TABLE_REACTIONS = {
  clap: {label: 'Aplausos', caption: 'Que espetáculo', glyph: '👏', targeted: false},
  laugh: {label: 'Risada', caption: 'Ri até doer', glyph: '😄', targeted: false},
  wow: {label: 'Uau', caption: 'Não acredito nisso', glyph: '😮', targeted: false},
  angry: {label: 'Raiva', caption: 'Tô fervendo aqui', glyph: '😤', targeted: false},
  cry: {label: 'Choro', caption: 'Doeu na alma', glyph: '😭', targeted: false},
  nervous: {label: 'Nervoso', caption: 'Suando frio', glyph: '😰', targeted: false},
  cold: {label: 'Frio na mesa', caption: 'Não entro em nada', glyph: '🥶', targeted: false},
  fire: {label: 'Pegando fogo', caption: 'Tô imparável', glyph: '🔥', targeted: false},
  respect: {label: 'Respeito', caption: 'Tiro o chapéu', glyph: '🫡', targeted: false},
  sleepy: {label: 'Sono', caption: 'Isso vai demorar', glyph: '🥱', targeted: false},
  heartbeat: {label: 'Coração all-in', caption: 'Tudo ou nada', glyph: '🫀', targeted: false},
  shark: {label: 'Modo tubarão', caption: 'Sangue na água', glyph: '🦈', targeted: false},
  pokerface: {label: 'Pokerface', caption: 'Nem tenta me ler', glyph: '😎', targeted: false},

  chip: {label: 'Jogar ficha', caption: 'Rico e sem talento', glyph: '🟠', targeted: true},
  coffee: {label: 'Mandar café', caption: 'Acorda pro jogo', glyph: '☕', targeted: true},
  clover: {label: 'Dar sorte', caption: 'Vai precisar de sorte', glyph: '🍀', targeted: true},
  horseshoe: {label: 'Jogar ferradura', caption: 'Só na sorte, né', glyph: '🧲', targeted: true},
  tear: {label: 'Jogar lágrima', caption: 'Chora mais, campeão', glyph: '💧', targeted: true},
  tomato: {label: 'Jogar tomate', caption: 'Toma na cara', glyph: '🍅', targeted: true},
  poop: {label: 'Jogar cocô', caption: 'Isso fede daqui', glyph: '💩', targeted: true},
  rofl: {label: 'Rir da cara', caption: 'Rindo de você', glyph: '🤣', targeted: true},
  duck: {label: 'Jogar pato', caption: 'Jogou igual pato', glyph: '🦆', targeted: true},
  turtle: {label: 'Chamar de lento', caption: 'Acorda, tá dormindo', glyph: '🐢', targeted: true},
  knife: {label: 'Jogar faca', caption: 'Pelas costas', glyph: '🗡️', targeted: true},
  flowers: {label: 'Mandar flores', caption: 'Meus pêsames', glyph: '💐', targeted: true},
  spotlight: {label: 'Boa leitura', caption: 'Te li fácil', glyph: '🔦', targeted: true},
  crown: {label: 'Passar a coroa', caption: 'Manda você então', glyph: '👑', targeted: true},
  bandage: {label: 'Curar bad beat', caption: 'Vai sobreviver, talvez', glyph: '🩹', targeted: true},
  cucumber: {label: 'Botar pepino', caption: 'Pulou que nem gato', glyph: '🥒', targeted: true},
  boomerang: {label: 'Jogar bumerangue', caption: 'Isso volta pra você', glyph: '🪃', targeted: true}
} as const;

export type TableReactionID = keyof typeof TABLE_REACTIONS;

export const PREMIUM_REACTION_IDS = new Set<TableReactionID>([
  'cold', 'fire', 'poop', 'rofl', 'knife', 'turtle'
]);

export function isTableReaction(value: string): value is TableReactionID {
  return value in TABLE_REACTIONS;
}

export interface TableReactionEvent {
  id: string;
  playerId: string;
  reactionId: TableReactionID;
  targetPlayerId?: string;
}
