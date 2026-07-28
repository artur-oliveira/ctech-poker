import {describe, expect, test} from 'vitest';
import {ServerMessage} from '@/lib/api/proto/poker';
import {decodeServerMessage, encodeClientMessage} from './utils';

describe('websocket protocol utilities', () => {
  test('encodes token-only legacy authentication as an auth message', () => {
    const encoded = encodeClientMessage({token: 'access-token'});
    expect(encoded).toBeInstanceOf(ArrayBuffer);
    expect(new Uint8Array(encoded).byteLength).toBeGreaterThan(0);
  });

  test('encodes explicitly typed client messages without replacing their type', () => {
    const encoded = encodeClientMessage({type: 'ping', token: 'token'});
    expect(encoded.byteLength).toBeGreaterThan(0);
  });

  test('decodes JSON compatibility frames', () => {
    expect(decodeServerMessage('{"type":"pong","sequence":2}' as unknown as object))
      .toEqual({type: 'pong', sequence: 2});
  });

  test('decodes protobuf binary frames', () => {
    const bytes = ServerMessage.encode(ServerMessage.fromPartial({type: 'pong'})).finish();
    const frame = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    expect(decodeServerMessage(frame)).toMatchObject({type: 'pong'});
  });

  test('surfaces malformed JSON instead of silently accepting corrupt frames', () => {
    expect(() => decodeServerMessage('not-json' as unknown as object)).toThrow();
  });
});
