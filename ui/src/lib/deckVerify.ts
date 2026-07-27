// Client-side replay of api/internal/engine/deck/deck.go's commit-reveal
// shuffle (OVERVIEW.md § 3.5), so a player can independently recompute the
// full 52-card deck from the hand's published commit hash + revealed server
// seed and confirm the two match, proving the deck wasn't altered mid-hand.
const SUITS = ['c', 'd', 'h', 's'] as const; // Clubs, Diamonds, Hearts, Spades, matching deck.go's iota order
const RANK_CHARS = ['2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'];

export interface DeckCard {
  rank: number; // 2-14, matches deck.go's Rank
  suit: number; // 0-3, matches deck.go's Suit
  code: string; // e.g. "Ah", consumable by lib/cards.ts / PlayingCard
}

function hexToBytes(hex: string): Uint8Array {
  const clean = hex.trim();
  const out = new Uint8Array(clean.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(clean.substring(i * 2, i * 2 + 2), 16);
  return out;
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
}

function orderedDeck(): DeckCard[] {
  const d: DeckCard[] = [];
  for (let s = 0; s < 4; s++) {
    for (let r = 2; r <= 14; r++) {
      d.push({rank: r, suit: s, code: `${RANK_CHARS[r - 2]}${SUITS[s]}`});
    }
  }
  return d;
}

const MAX_U32 = 0xffffffff;

// Fisher-Yates driven by an HMAC-SHA256 byte stream keyed on the seed.
// Deterministic (same seed -> same permutation), with rejection sampling to
// avoid modulo bias, mirroring shuffleWithSeed in deck.go exactly, including
// consuming a counter tick on every rejected draw.
export async function shuffleWithSeed(seedHex: string): Promise<DeckCard[]> {
  const d = orderedDeck();
  const keyfmt: KeyFormat = 'raw';
  const key = await crypto.subtle.importKey(keyfmt, hexToBytes(seedHex) as BufferSource, {name: 'HMAC', hash: 'SHA-256'}, false, ['sign']);
  let counter = 0;

  async function nextIndex(max: number): Promise<number> {
    for (;;) {
      const ctrBytes = new Uint8Array(4);
      new DataView(ctrBytes.buffer).setUint32(0, counter, false);
      counter++;
      const sig = new Uint8Array(await crypto.subtle.sign('HMAC', key, ctrBytes));
      const v = new DataView(sig.buffer).getUint32(0, false);
      const rem = (MAX_U32 % max + 1) % max;
      const limit = MAX_U32 - rem + 1;
      if (rem === 0 || v < limit) return v % max;
    }
  }

  for (let i = d.length - 1; i > 0; i--) {
    const j = await nextIndex(i + 1);
    [d[i], d[j]] = [d[j], d[i]];
  }
  return d;
}

export async function commitHash(seedHex: string, cards: DeckCard[]): Promise<string> {
  const bytes = new Uint8Array(32 + cards.length * 2);
  bytes.set(hexToBytes(seedHex), 0);
  cards.forEach((c, i) => {
    bytes[32 + i * 2] = c.rank;
    bytes[32 + i * 2 + 1] = c.suit;
  });
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes as BufferSource));
  return bytesToHex(digest);
}

export interface VerifyResult {
  deck: DeckCard[];
  computedHash: string;
  matches: boolean;
}

// Replays the shuffle from the revealed seed and checks it against the
// commit hash published before the hand: the entire point of the exercise.
export async function verifyDeck(serverSeedHex: string, commitHashHex: string): Promise<VerifyResult> {
  const deck = await shuffleWithSeed(serverSeedHex);
  const computedHash = await commitHash(serverSeedHex, deck);
  return {deck, computedHash, matches: computedHash.toLowerCase() === commitHashHex.trim().toLowerCase()};
}
