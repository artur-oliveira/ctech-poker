import {LobbyClientMessage, LobbyServerMessage} from "@/lib/api/proto/lobby";
import {decodeWith, encodeWith} from "@/lib/ws/codec";

/**
 * The **lobby/user gateway** codec: `lobby.proto`, which mirrors the field
 * numbers of `poker.ServerMessage` but stops before `TableSnapshot`. Same
 * bytes, same frames — everything only the table reads is skipped as an
 * unknown field, and the ~150 kB of generated snapshot codec stays off the
 * critical path of Lobby, Store, Profile, Hands, Achievements and People
 * (#228). Table code imports `./utils.ts` instead.
 */

export const encodeLobbyClientMessage = (val: object): ArrayBuffer => encodeWith(LobbyClientMessage, val);

export const decodeLobbyServerMessage = (data: object): object => decodeWith(LobbyServerMessage, data);
