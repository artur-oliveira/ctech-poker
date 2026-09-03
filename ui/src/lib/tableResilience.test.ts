import assert from 'node:assert/strict';
import {test} from 'vitest';
import {
  actionError, auxRetryDelayMs, AUX_RETRY_BASE_MS, MAX_ACTION_RETRIES,
  RESYNC_ERROR_CODES, RESYNC_TIMEOUT_MS, TERMINAL_ERROR_CODES
} from './tableResilience.ts';

test('a rejection that means "your view is not the server\'s view" is resubmitted, not surfaced', () => {
  for (const code of ['stale_state', 'rate_limited', 'invalid_action', 'unavailable']) {
    assert.equal(RESYNC_ERROR_CODES.has(code), true, code);
    assert.equal(TERMINAL_ERROR_CODES.has(code), false, code);
  }
});

test('only access and existence failures are terminal', () => {
  assert.deepEqual([...TERMINAL_ERROR_CODES], ['forbidden', 'not_found']);
  // A terminal code must never also schedule a resync: the table is gone.
  for (const code of TERMINAL_ERROR_CODES) assert.equal(RESYNC_ERROR_CODES.has(code), false, code);
});

test('the aux resubmit backs off past the resync scheduled for the same action_id', () => {
  // The resync for a first rejection lands well inside RESYNC_TIMEOUT_MS, so
  // the first resubmit must be judged against the state that resync pulled.
  assert.equal(auxRetryDelayMs(1, 0), AUX_RETRY_BASE_MS);
  assert.ok(auxRetryDelayMs(1, 0) < RESYNC_TIMEOUT_MS);
  assert.equal(auxRetryDelayMs(2, 0), AUX_RETRY_BASE_MS * 2);
  assert.equal(auxRetryDelayMs(3, 0), AUX_RETRY_BASE_MS * 4);
});

test('jitter widens the delay without ever shortening it', () => {
  assert.equal(auxRetryDelayMs(1, 0.999), AUX_RETRY_BASE_MS + 199);
  const delay = auxRetryDelayMs(2);
  assert.ok(delay >= AUX_RETRY_BASE_MS * 2 && delay < AUX_RETRY_BASE_MS * 2 + 200);
});

test('the retry budget is finite, so a genuinely illegal action fails visibly', () => {
  assert.equal(MAX_ACTION_RETRIES, 3);
  // Worst case the player waits out the whole ramp before seeing the error.
  const total = Array.from({length: MAX_ACTION_RETRIES}, (_, index) => auxRetryDelayMs(index + 1, 1));
  assert.deepEqual(total.map(Math.round), [900, 1600, 3000]);
});

test('every rejection the player can see carries copy, and the rest fall back', () => {
  for (const code of [...RESYNC_ERROR_CODES, ...TERMINAL_ERROR_CODES, 'unauthorized', 'action_timeout']) {
    const error = actionError(code);
    assert.equal(error.code, code);
    assert.notEqual(error.message, 'Não foi possível concluir a ação. Tente novamente.');
  }
  assert.deepEqual(actionError(), {
    code: 'unknown', message: 'Não foi possível concluir a ação. Tente novamente.'
  });
  assert.equal(actionError('brand_new_server_code').message,
    'Não foi possível concluir a ação. Tente novamente.');
});
