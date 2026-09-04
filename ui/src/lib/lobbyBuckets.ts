// One lobby bucket — the (blinds, seats) pick a player makes in the lobby —
// carried from the stakes grid to the buy-in ceremony on the table route.
// The room id is deliberately absent: `POST /rooms/join-or-create` resolves
// which table the player lands in, server-side, at buy-in time (#205).
export type LobbyBucket = {
  smallBlind: number;
  bigBlind: number;
  maxSeats: number;
};

export const ROOM_BUCKETS_QUERY_KEY = ['room-buckets', 'sandbox'] as const;

// The lobby's quick-join is sandbox-only, same as the grid it comes from.
export const LOBBY_BUCKET_SEATS = [2, 6, 9] as const;

export function tableBucketHref(bucket: LobbyBucket) {
  return `/table?sb=${bucket.smallBlind}&bb=${bucket.bigBlind}&seats=${bucket.maxSeats}`;
}

/** The bucket a `/table` URL carries, or null when it is not a bucket entry.
 * Rejects anything the join-or-create endpoint would refuse outright, so an
 * edited URL lands on the invalid-table screen instead of a failed buy-in. */
export function bucketFromParams(params: URLSearchParams | null): LobbyBucket | null {
  if (!params) return null;
  const smallBlind = Number(params.get('sb'));
  const bigBlind = Number(params.get('bb'));
  const maxSeats = Number(params.get('seats'));
  const valid = Number.isInteger(smallBlind) && smallBlind > 0
    && Number.isInteger(bigBlind) && bigBlind > smallBlind
    && (LOBBY_BUCKET_SEATS as readonly number[]).includes(maxSeats);
  return valid ? {smallBlind, bigBlind, maxSeats} : null;
}
