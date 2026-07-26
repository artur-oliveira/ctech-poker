import {ClientMessage, ServerMessage} from "@/lib/api/proto/poker";

/**
 * Encodes outbound client messages into binary frames.
 */
export const encodeClientMessage = (val: object): Uint8Array => {
  const msg = ClientMessage.fromPartial(val);
  if ((val as { token: string }).token && !msg.type) {
    msg.type = 'auth';
  }
  return ClientMessage.encode(msg).finish();
};

/**
 * Decodes inbound binary frames from the server into ServerMessage structures.
 */
export const decodeServerMessage = (data: object): object => {
  if (typeof data === 'string') {
    return JSON.parse(data);
  }
  const bytes = new Uint8Array(data as ArrayBuffer);
  const msg = ServerMessage.decode(bytes);
  return msg;
};
