import {cardHashHex, cardSaltHex, rootCommitHash, shuffleWithSeed} from '@/lib/deckVerify';

/** Builds the same viewer-scoped partial-deck proof the server sends
 * (snapshot.go's fairnessProofFor) from a known seed: every position in
 * `reveal` comes out as card + salt, every other one as its committed hash.
 * Mock runtime and tests only — the real seed never reaches a browser while a
 * card is still hidden. */
export async function mockPartialDeckProof(seedHex: string, reveal: readonly number[]) {
  const deck = await shuffleWithSeed(seedHex);
  const revealed: Record<number, { card: string; salt_hex: string }> = {};
  const unrevealed: Record<number, string> = {};
  const hashes: string[] = [];
  for (let i = 0; i < deck.length; i++) {
    const salt = await cardSaltHex(seedHex, i);
    hashes.push(await cardHashHex(salt, deck[i].rank, deck[i].suit));
    if (reveal.includes(i)) revealed[i] = {card: deck[i].code, salt_hex: salt};
    else unrevealed[i] = hashes[i];
  }
  return {deck, root: await rootCommitHash(hashes), revealed, unrevealed};
}
