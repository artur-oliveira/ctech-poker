import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {describe, expect, test} from 'vitest';
import {type DeepPartial, ServerMessage} from '@/lib/api/proto/poker';
import {LobbyClientMessage} from '@/lib/api/proto/lobby';
import {decodeLobbyServerMessage, encodeLobbyClientMessage} from './lobbyCodec';

/** The server always encodes the full `poker.ServerMessage`; the lobby decodes
 *  the same bytes with the subset codec. Every test here therefore *produces*
 *  its input with the full codec — a subset that stopped agreeing on a field
 *  number would fail on the wire, not just in types. */
const frameFrom = (value: DeepPartial<ServerMessage>) => {
  const bytes = ServerMessage.encode(ServerMessage.fromPartial(value)).finish();
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
};

describe('lobby websocket codec', () => {
  test('decodes the lobby frames off a full ServerMessage', () => {
    expect(decodeLobbyServerMessage(frameFrom({
      type: 'room_updated', room_id: 'room-7', seats_taken: 4,
    }))).toMatchObject({type: 'room_updated', room_id: 'room-7', seats_taken: 4});

    expect(decodeLobbyServerMessage(frameFrom({
      type: 'social_event',
      social_event: {event_id: 'evt-1', type: 'table_invite', room_id: 'room-9', actor_id: 'zoe'},
    }))).toMatchObject({
      type: 'social_event',
      social_event: {event_id: 'evt-1', type: 'table_invite', room_id: 'room-9'},
    });

    expect(decodeLobbyServerMessage(frameFrom({
      type: 'sandbox_purchase_update', code: 'confirmed', purchase_id: 'p-1', amount: 5000,
    }))).toMatchObject({type: 'sandbox_purchase_update', code: 'confirmed', purchase_id: 'p-1', amount: 5000});

    expect(decodeLobbyServerMessage(frameFrom({type: 'social_inbox_count', unread_count: 3})))
      .toMatchObject({type: 'social_inbox_count', unread_count: 3});

    expect(decodeLobbyServerMessage(frameFrom({type: 'system_broadcast', text: 'manutenção às 3h'})))
      .toMatchObject({type: 'system_broadcast', text: 'manutenção às 3h'});
  });

  test('room_created keeps a present/absent room marker without decoding the room', () => {
    expect(decodeLobbyServerMessage(frameFrom({
      type: 'room_created', room: {room_id: 'room-1', big_blind: 200},
    }))).toMatchObject({type: 'room_created', room: {}});
    // Absent stays undefined, exactly as the full codec reports it — the lobby
    // branches on `message.room` being there.
    expect(decodeLobbyServerMessage(frameFrom({type: 'room_created'})))
      .toMatchObject({type: 'room_created', room: undefined});
  });

  test('skips a table snapshot instead of choking on it', () => {
    const frame = frameFrom({
      type: 'state',
      snapshot: {stage: 'flop', board: ['As', 'Kd', '7h'], seats: [{player_id: 'zoe', name: 'Zoe', stack: 1200}]},
    });
    expect(decodeLobbyServerMessage(frame)).toMatchObject({type: 'state'});
  });

  test('encodes the two frames the gateway sends, inferring the legacy auth type', () => {
    const auth = encodeLobbyClientMessage({token: 'access-token'});
    expect(LobbyClientMessage.decode(new Uint8Array(auth)))
      .toMatchObject({type: 'auth', token: 'access-token'});
    const ping = encodeLobbyClientMessage({type: 'ping'});
    expect(LobbyClientMessage.decode(new Uint8Array(ping))).toMatchObject({type: 'ping'});
  });

  test('decodes JSON compatibility frames and surfaces malformed ones', () => {
    expect(decodeLobbyServerMessage('{"type":"pong"}' as unknown as object)).toEqual({type: 'pong'});
    expect(() => decodeLobbyServerMessage('not-json' as unknown as object)).toThrow();
  });
});

/** The reason the subset exists: `poker.ts` is ~150 kB of generated codec and
 *  the lobby gateway is mounted by the `(app)` layout, i.e. on Store, Profile,
 *  Hands, Achievements and People too. Walking the static import graph is what
 *  keeps an innocent-looking `import type {Seat}` from putting it back. */
describe('lobby gateway module graph', () => {
  const SRC = resolve(__dirname, '..', '..');

  function resolveImport(specifier: string, fromFile: string) {
    const base = specifier.startsWith('@/') ? join(SRC, specifier.slice(2))
      : specifier.startsWith('.') ? resolve(dirname(fromFile), specifier)
        : null;
    if (!base) return null;
    for (const candidate of [`${base}.ts`, `${base}.tsx`, join(base, 'index.ts')]) {
      try {
        readFileSync(candidate);
        return candidate;
      } catch {
        // not this extension
      }
    }
    return null;
  }

  test('never reaches the full table codec', () => {
    const seen = new Set<string>();
    const queue = [join(SRC, 'lib/providers/RealtimeBridge.tsx')];
    while (queue.length) {
      const file = queue.pop()!;
      if (seen.has(file)) continue;
      seen.add(file);
      const source = readFileSync(file, 'utf8');
      for (const [, specifier] of source.matchAll(/from\s+['"]([^'"]+)['"]/g)) {
        // A type-only import is erased by the compiler and costs no bytes.
        if (new RegExp(`import\\s+type[^;]*?from\\s+['"]${specifier.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}['"]`)
          .test(source)) continue;
        const target = resolveImport(specifier, file);
        if (target) queue.push(target);
      }
    }
    expect([...seen].some(file => file.endsWith('api/proto/poker.ts'))).toBe(false);
    // Sanity: the walk actually walked, and found the codec it is supposed to use.
    expect([...seen].some(file => file.endsWith('ws/lobbyCodec.ts'))).toBe(true);
  });
});
