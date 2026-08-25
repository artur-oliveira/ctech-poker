export const TABLE_REACTIONS = {
  clap: {label: 'Aplausos', caption: 'Bela jogada', glyph: '👏', targeted: false},
  laugh: {label: 'Risada', caption: 'Essa foi boa', glyph: '😄', targeted: false},
  wow: {label: 'Uau', caption: 'Que virada', glyph: '😮', targeted: false},
  angry: {label: 'Raiva', caption: 'Tilt controlado', glyph: '😤', targeted: false},
  cry: {label: 'Choro', caption: 'Bad beat doeu', glyph: '😭', targeted: false},
  nervous: {label: 'Nervoso', caption: 'Pote gigante', glyph: '😰', targeted: false},
  cold: {label: 'Frio na mesa', caption: 'Mesa congelada', glyph: '🥶', targeted: false},
  fire: {label: 'Sequência quente', caption: 'Pegando fogo', glyph: '🔥', targeted: false},
  respect: {label: 'Respeito', caption: 'Jogada de mestre', glyph: '🫡', targeted: false},
  sleepy: {label: 'Sono', caption: 'Pensando há eras', glyph: '🥱', targeted: false},
  heartbeat: {label: 'Coração all-in', caption: 'Tudo no centro', glyph: '🫀', targeted: false},
  shark: {label: 'Modo tubarão', caption: 'Caçando o pote', glyph: '🦈', targeted: false},
  pokerface: {label: 'Cara de pôquer', caption: 'Nada para entregar', glyph: '😎', targeted: false},

  chip: {label: 'Jogar ficha', caption: 'Uma ficha no alvo', glyph: '🟠', targeted: true},
  coffee: {label: 'Mandar café', caption: 'Para voltar ao jogo', glyph: '☕', targeted: true},
  clover: {label: 'Dar sorte', caption: 'Run good para você', glyph: '🍀', targeted: true},
  horseshoe: {label: 'Jogar ferradura', caption: 'Sorte em dobro', glyph: '🧲', targeted: true},
  tear: {label: 'Jogar lágrima', caption: 'Uma lágrima só', glyph: '💧', targeted: true},
  tomato: {label: 'Jogar tomate', caption: 'Direto no feltro', glyph: '🍅', targeted: true},
  poop: {label: 'Jogar cocô', caption: 'Mão complicada', glyph: '💩', targeted: true},
  rofl: {label: 'Rir da cara', caption: 'Não deu para segurar', glyph: '🤣', targeted: true},
  duck: {label: 'Jogar pato', caption: 'Visita inesperada', glyph: '🦆', targeted: true},
  turtle: {label: 'Chamar de lento', caption: 'Entrou no tanque', glyph: '🐢', targeted: true},
  knife: {label: 'Jogar faca', caption: 'Corte limpo', glyph: '🗡️', targeted: true},
  flowers: {label: 'Mandar flores', caption: 'Respeito na mesa', glyph: '💐', targeted: true},
  spotlight: {label: 'Boa leitura', caption: 'Você enxergou tudo', glyph: '🔦', targeted: true},
  crown: {label: 'Passar a coroa', caption: 'Dono desta mão', glyph: '👑', targeted: true},
  bandage: {label: 'Curar bad beat', caption: 'Sobrevive à próxima', glyph: '🩹', targeted: true}
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
