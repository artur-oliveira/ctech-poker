export const TABLE_REACTIONS = {
  clap: {label: 'Aplausos', caption: 'Palmas. Bem devagar.', glyph: '👏', targeted: false},
  laugh: {label: 'Risada', caption: 'Kkkkk... tô sem ar', glyph: '😄', targeted: false},
  wow: {label: 'Uau', caption: 'Você jogou isso mesmo?', glyph: '😮', targeted: false},
  angry: {label: 'Raiva', caption: 'Tô a um bad beat de surtar', glyph: '😤', targeted: false},
  cry: {label: 'Choro', caption: 'Chora, mas paga o blind', glyph: '😭', targeted: false},
  nervous: {label: 'Nervoso', caption: 'Tá tudo sob controle', glyph: '😰', targeted: false},
  cold: {label: 'Frio na mesa', caption: 'Mesa virou geladeira', glyph: '🥶', targeted: false},
  fire: {label: 'Pegando fogo', caption: 'Chama o bombeiro', glyph: '🔥', targeted: false},
  respect: {label: 'Respeito', caption: 'F pelo stack caído', glyph: '🫡', targeted: false},
  sleepy: {label: 'Sono', caption: 'Acorda quando subir o blind', glyph: '🥱', targeted: false},
  heartbeat: {label: 'Coração all-in', caption: 'Reza que o coração aguenta', glyph: '🫀', targeted: false},
  shark: {label: 'Modo tubarão', caption: 'Tubarão não dorme, blefa', glyph: '🦈', targeted: false},
  pokerface: {label: 'Pokerface', caption: 'Blefe ou nuts? Adivinha.', glyph: '😎', targeted: false},

  chip: {label: 'Jogar ficha', caption: 'Toma, vai comprar talento', glyph: '🟠', targeted: true},
  coffee: {label: 'Mandar café', caption: 'Café pra quem dormiu', glyph: '☕', targeted: true},
  clover: {label: 'Dar sorte', caption: 'Te abençoo. Vai precisar', glyph: '🍀', targeted: true},
  horseshoe: {label: 'Jogar ferradura', caption: 'Ímã de ficha, hein?', glyph: '🧲', targeted: true},
  tear: {label: 'Jogar lágrima', caption: 'Chora mais, eu aguento', glyph: '💧', targeted: true},
  tomato: {label: 'Jogar tomate', caption: 'Esse blefe pede molho', glyph: '🍅', targeted: true},
  poop: {label: 'Jogar cocô', caption: 'Chegou o delivery', glyph: '💩', targeted: true},
  rofl: {label: 'Rir da cara', caption: 'Sua jogada é piada pronta', glyph: '🤣', targeted: true},
  duck: {label: 'Jogar pato', caption: 'Pato confirmado', glyph: '🦆', targeted: true},
  turtle: {label: 'Chamar de lento', caption: 'Tá no timebank desde ontem', glyph: '🐢', targeted: true},
  knife: {label: 'Jogar faca', caption: 'Nada pessoal, parça', glyph: '🗡️', targeted: true},
  flowers: {label: 'Mandar flores', caption: 'Foi bonito enquanto durou', glyph: '💐', targeted: true},
  spotlight: {label: 'Boa leitura', caption: 'Tá escrito na sua testa', glyph: '🔦', targeted: true},
  crown: {label: 'Passar a coroa', caption: 'Agora o problema é seu, rei', glyph: '👑', targeted: true},
  bandage: {label: 'Curar bad beat', caption: 'Passa com beijinho', glyph: '🩹', targeted: true},
  cucumber: {label: 'Botar pepino', caption: 'Olha atrás, gatinho', glyph: '🥒', targeted: true},
  boomerang: {label: 'Jogar bumerangue', caption: 'Vai, mas volta com juros', glyph: '🪃', targeted: true}
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

/** Required by CC BY 4.0 for the animated Noto emoji bundled in public/emoji. */
export const NOTO_CREDIT = 'Emojis: Noto Emoji, © Google, CC BY 4.0';
export const NOTO_CREDIT_URL = 'https://googlefonts.github.io/noto-emoji-animation/';

/** Mirrors `maxFavoritesPerKind` in api/internal/player/service.go: up to three
 * favorites on the player's own seat and up to three thrown at someone. */
export const MAX_FAVORITES_PER_KIND = 3;

/** The number keys the quick bar owns. 1 and 2 peek the hole cards, so the
 * favorites take 3 to 8: self reactions first (3-5), then thrown ones (6-8). */
export const FAVORITE_KEYS = ['3', '4', '5', '6', '7', '8'] as const;

export interface FavoriteSlot {
  id: TableReactionID;
  key: typeof FAVORITE_KEYS[number];
}

/** Self favorites sit on 3-5 and thrown ones on 6-8 whatever the counts, so a
 * key never changes meaning because the player starred one more of the other
 * kind. Within a kind, the order they were starred in is the key order. */
export function favoriteSlots(favorites: TableReactionID[]): FavoriteSlot[] {
  const pick = (targeted: boolean, offset: number) => favorites
    .filter(id => TABLE_REACTIONS[id].targeted === targeted)
    .slice(0, MAX_FAVORITES_PER_KIND)
    .map((id, index) => ({id, key: FAVORITE_KEYS[offset + index]}));
  return [...pick(false, 0), ...pick(true, MAX_FAVORITES_PER_KIND)];
}
