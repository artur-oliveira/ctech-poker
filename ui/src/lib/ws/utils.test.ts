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

  // The decoded proto Seat IS the SeatView (no mapping layer), so the four
  // cosmetic fields arrive under their proto names; absent ones stay unset.
  test('decodes the seat cosmetics under their proto names', () => {
    const bytes = ServerMessage.encode(ServerMessage.fromPartial({type: 'state', snapshot: {seats: [
      {player_id: 'a', frame_id: 'chip-ring', hat_id: 'crown', accessory_id: 'monocle', badge_ids: ['veteran']},
      {player_id: 'b'},
    ]}})).finish();
    const frame = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    const [a, b] = (decodeServerMessage(frame) as {snapshot: {seats: Array<Record<string, unknown>>}}).snapshot.seats;
    expect(a).toMatchObject({frame_id: 'chip-ring', hat_id: 'crown', accessory_id: 'monocle', badge_ids: ['veteran']});
    expect(b.frame_id).toBeUndefined();
    expect(b.hat_id).toBeUndefined();
    expect(b.accessory_id).toBeUndefined();
    expect(b.badge_ids).toEqual([]);
  });
});
