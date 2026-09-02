import {describe, expect, test} from 'vitest';
import {metadata} from './layout';

describe('store metadata', () => {
  test('identifies the store instead of inheriting the landing title', () => {
    expect(metadata.title).toBe('Loja');
    expect(metadata.alternates).toEqual({canonical: '/store'});
  });
});
