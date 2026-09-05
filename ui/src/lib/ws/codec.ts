/** The wire framing, once, for both sockets.
 *
 * The table and the lobby/user gateway send the same bytes but decode
 * different halves of them: `./utils.ts` binds these to the full
 * `poker.ServerMessage` (snapshot included) and `./lobbyCodec.ts` to the
 * gateway subset, so the heavy generated codec stays off every non-table
 * route's critical path (#228). Only the framing — the `auth` inference, the
 * ArrayBuffer slice, the JSON compatibility branch — lives here, so the two
 * bindings cannot drift apart. */

interface Codec<T> {
  fromPartial(value: object): T;

  encode(message: T): {finish(): Uint8Array};

  decode(bytes: Uint8Array): T;
}

/** Encodes outbound client messages into binary frames. */
export function encodeWith<T extends {type: string}>(codec: Codec<T>, val: object): ArrayBuffer {
  const msg = codec.fromPartial(val);
  if ((val as {token?: string}).token && !msg.type) {
    msg.type = 'auth';
  }
  const bytes = codec.encode(msg).finish();
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

/** Decodes inbound binary frames from the server into message structures. */
export function decodeWith<T>(codec: Codec<T>, data: object): object {
  if (typeof data === 'string') {
    return JSON.parse(data);
  }
  return codec.decode(new Uint8Array(data as ArrayBuffer)) as object;
}
