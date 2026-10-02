import type {TableSnapshot} from '@/lib/api/table';
import {parseCardCode, verifyWirePartialDeck} from '@/lib/deckVerify';

/** The protocol version that sells the rabbit hunt one board slot at a time
 * (`board_slot`, `rabbit_cards`, `rabbit_hunt_fee`). Older servers sell the
 * whole runout for the big blind, which this client no longer offers. */
export const PER_CARD_REVEALS_PROTOCOL = 12;

// Offset of each board slot past the hole cards in the shuffled deck: the
// flop follows one burn, the turn and the river one burn each. Mirrors
// api/internal/engine/hand/snapshot.go's boardSlotDeckIndex.
const SLOT_OFFSETS = [1, 2, 3, 5, 7];

export function rabbitDeckIndex(dealtPlayers: number, slot: number) {
  return dealtPlayers * 2 + SLOT_OFFSETS[slot];
}

/** The per-card price while the rabbit hunt is on offer to `viewer`, else 0.
 * Gated on the deck *commitment*, never on the proof: the server withholds
 * every runout card until it is bought, so requiring the proof would make the
 * offer depend on data only the purchase produces. */
export function rabbitHuntFee(snapshot: TableSnapshot, viewer?: string) {
  if ((snapshot.protocol_version ?? 0) < PER_CARD_REVEALS_PROTOCOL) return 0;
  const dealtIn = snapshot.seats.some(seat => seat.player_id === viewer && seat.dealt_in);
  const committed = Boolean(snapshot.shuffle_commit_hash || snapshot.root_commit_hash);
  if (snapshot.stage !== 'complete' || !snapshot.won_without_showdown || snapshot.board.length >= 5 ||
    !committed || !dealtIn) return 0;
  return snapshot.rabbit_hunt_fee ?? 0;
}

/** Checks one bought card in the browser: the partial-deck proof must rebuild
 * the published root commit, and the salted card at that slot's deck position
 * must be the card the server says is there. */
export async function verifyRabbitCard(snapshot: TableSnapshot, slot: number) {
  const card = snapshot.rabbit_cards?.[slot];
  const {root_commit_hash: root, revealed_card_salts: salts, unrevealed_card_hashes: hashes} = snapshot;
  if (!card || !root || !salts || !hashes) return false;
  const dealtPlayers = snapshot.seats.filter(seat => seat.dealt_in).length;
  const revealed = salts[rabbitDeckIndex(dealtPlayers, slot)];
  if (!revealed || parseCardCode(revealed.card).code !== parseCardCode(card).code) return false;
  return (await verifyWirePartialDeck(root, salts, hashes)).matches;
}
