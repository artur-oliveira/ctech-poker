/**
 * Frontend timing fallbacks and development-mock timings.
 *
 * The Go table actor is authoritative in production and sends absolute
 * deadlines in every snapshot. These values cover an old/missing room field,
 * hand replay, and mock mode. When changing the product defaults, keep them in
 * sync with api/internal/table/turntimeout.go.
 */
export const DEFAULT_TURN_TIMEOUT_SECONDS = 15;
export const DEFAULT_TURN_TIMEOUT_MS = DEFAULT_TURN_TIMEOUT_SECONDS * 1_000;
export const NEXT_HAND_DELAY_SECONDS = 12;
export const NEXT_HAND_DELAY_MS = NEXT_HAND_DELAY_SECONDS * 1_000;
