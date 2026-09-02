import {beforeEach, describe, expect, test, vi} from 'vitest';
import {clearPersistedHandShare, getPersistedHandShare, setPersistedHandShare} from './handShareStorage';

const STORAGE_KEY = 'ctech-poker:hand-shares:v1';

beforeEach(() => {
  window.localStorage.clear();
});

describe('handShareStorage', () => {
  test('returns null when nothing was persisted for a hand', () => {
    expect(getPersistedHandShare('hand-1')).toBeNull();
  });

  test('round-trips a persisted share for its hand id', () => {
    setPersistedHandShare('hand-1', {token: 'tok-1', expiresAt: Date.now() + 60_000});
    expect(getPersistedHandShare('hand-1')).toEqual({token: 'tok-1', expiresAt: expect.any(Number)});
    expect(getPersistedHandShare('hand-2')).toBeNull();
  });

  test('treats an expired local record as absent', () => {
    setPersistedHandShare('hand-1', {token: 'tok-1', expiresAt: Date.now() - 1});
    expect(getPersistedHandShare('hand-1')).toBeNull();
  });

  test('clearing removes only the targeted hand', () => {
    setPersistedHandShare('hand-1', {token: 'tok-1', expiresAt: Date.now() + 60_000});
    setPersistedHandShare('hand-2', {token: 'tok-2', expiresAt: Date.now() + 60_000});
    clearPersistedHandShare('hand-1');
    expect(getPersistedHandShare('hand-1')).toBeNull();
    expect(getPersistedHandShare('hand-2')).not.toBeNull();
  });

  test('clearing a hand with no record is a no-op', () => {
    expect(() => clearPersistedHandShare('missing')).not.toThrow();
  });

  test('ignores malformed stored JSON', () => {
    window.localStorage.setItem(STORAGE_KEY, '{not json');
    expect(getPersistedHandShare('hand-1')).toBeNull();
  });

  test('ignores a stored value that is not an object', () => {
    window.localStorage.setItem(STORAGE_KEY, '"just a string"');
    expect(getPersistedHandShare('hand-1')).toBeNull();
  });

  test('drops entries that are missing token or expiresAt', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({
      'hand-1': {token: 'tok-1'},
      'hand-2': {expiresAt: Date.now() + 60_000},
      'hand-3': null,
    }));
    expect(getPersistedHandShare('hand-1')).toBeNull();
    expect(getPersistedHandShare('hand-2')).toBeNull();
    expect(getPersistedHandShare('hand-3')).toBeNull();
  });

  test('swallows storage write failures', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded');
    });
    try {
      expect(() => setPersistedHandShare('hand-1', {token: 'tok-1', expiresAt: Date.now() + 60_000})).not.toThrow();
    } finally {
      spy.mockRestore();
    }
  });
});
