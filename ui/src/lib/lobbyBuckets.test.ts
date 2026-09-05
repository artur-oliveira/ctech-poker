import {describe, expect, test} from 'vitest';
import {bucketFromParams, tableBucketHref} from './lobbyBuckets';

describe('lobby bucket URLs', () => {
  test('round-trips a pick from the lobby to the table route', () => {
    const bucket = {smallBlind: 25, bigBlind: 50, maxSeats: 6};
    const href = tableBucketHref(bucket);
    expect(href).toBe('/table?sb=25&bb=50&seats=6');
    expect(bucketFromParams(new URLSearchParams(href.split('?')[1]))).toEqual(bucket);
  });

  test('rejects anything join-or-create would refuse', () => {
    const cases = [
      '', 'sb=25', 'sb=25&bb=50', 'sb=25&bb=50&seats=4', 'sb=0&bb=50&seats=6',
      'sb=50&bb=50&seats=6', 'sb=25.5&bb=50&seats=6', 'sb=abc&bb=50&seats=6',
    ];
    for (const query of cases) {
      expect(bucketFromParams(new URLSearchParams(query))).toBeNull();
    }
    expect(bucketFromParams(null)).toBeNull();
  });
});
