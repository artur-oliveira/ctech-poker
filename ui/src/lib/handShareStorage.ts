// Local record of hand shares the viewer created in *this* browser. There is
// no `GET /players/me/hand-shares` yet (backend #77), so this is the only way
// the client can know "I already shared this hand" when `ShareHandDialog`
// reopens for the same `handId` — it is best-effort (another device, or a
// cleared browser, will not see it) and never asserts anything about the
// share's live status beyond a client-side expiry guess.
const STORAGE_KEY = 'ctech-poker:hand-shares:v1';

export interface PersistedHandShare {
  token: string;
  expiresAt: number;
}

type StoredMap = Record<string, PersistedHandShare>;

function readAll(): StoredMap {
  if (typeof window === 'undefined') return {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== 'object') return {};
    const result: StoredMap = {};
    for (const [handId, value] of Object.entries(parsed as Record<string, unknown>)) {
      const entry = value as Partial<PersistedHandShare> | null;
      if (entry && typeof entry.token === 'string' && typeof entry.expiresAt === 'number') {
        result[handId] = {token: entry.token, expiresAt: entry.expiresAt};
      }
    }
    return result;
  } catch {
    return {};
  }
}

function writeAll(map: StoredMap) {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    // Storage unavailable (private mode, quota) — the share still works this
    // session, it just won't be remembered on reopen.
  }
}

/** Returns the locally-remembered share for a hand, or `null` if there isn't
 * one or its client-side expiry has already passed. */
export function getPersistedHandShare(handId: string): PersistedHandShare | null {
  const entry = readAll()[handId];
  if (!entry) return null;
  if (entry.expiresAt <= Date.now()) return null;
  return entry;
}

export function setPersistedHandShare(handId: string, share: PersistedHandShare) {
  const map = readAll();
  map[handId] = share;
  writeAll(map);
}

export function clearPersistedHandShare(handId: string) {
  const map = readAll();
  if (!(handId in map)) return;
  delete map[handId];
  writeAll(map);
}
