import {ClientMessage, ServerMessage} from "@/lib/api/proto/poker";
import {decodeWith, encodeWith} from "@/lib/ws/codec";

/**
 * The **table** codec: the full `poker.proto` contract, snapshot included.
 * Importing it pulls the whole generated protobuf module, so it belongs to the
 * table surface only — the lobby/user gateway uses `./lobbyCodec.ts` (#228).
 */

/**
 * Encodes outbound client messages into binary frames.
 */
export const encodeClientMessage = (val: object): ArrayBuffer => encodeWith(ClientMessage, val);

/**
 * Decodes inbound binary frames from the server into ServerMessage structures.
 */
export const decodeServerMessage = (data: object): object => decodeWith(ServerMessage, data);
