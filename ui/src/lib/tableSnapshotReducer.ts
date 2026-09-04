import type {TableSnapshot} from '@/lib/api/table';
import {CHAT_HISTORY_LIMIT} from '@/lib/chat';
import {isTableReaction, type TableReactionEvent, type TableReactionID} from '@/lib/reactions';

export type SnapshotChat = {id: string; player: string; message: string; timestamp?: number};
export type SnapshotView = {
  snapshot: TableSnapshot;
  version: number;
  handId: string;
  protocolVersion: number;
  chat: SnapshotChat[];
  reactions: Array<TableReactionEvent & {expiresAt: number}>;
};

/** Pure snapshot boundary. It rejects regressive versions and derives only
 * displayable social state, applying mute/block suppression before those
 * events can enter React state. Poker state is preserved byte-for-byte. */
export function reduceTableSnapshot(snapshot: TableSnapshot, latestVersion: number,
  suppressed: ReadonlySet<string> | undefined, now: number): SnapshotView | null {
  const version = snapshot.snapshot_version ?? 0;
  if (latestVersion >= 0 && version < latestVersion) return null;
  const protocolVersion = snapshot.protocol_version ?? 0;
  const chat = protocolVersion >= 6 ? (snapshot.chat_messages ?? [])
    .filter(item => !suppressed?.has(item.player_id))
    .map(item => ({id: item.id, player: item.player_id, message: item.message, timestamp: item.timestamp}))
    .slice(-CHAT_HISTORY_LIMIT) : [];
  const reactions = protocolVersion >= 6 ? (snapshot.reactions ?? [])
    .filter(item => isTableReaction(item.reaction_id) && item.expires_at > now && !suppressed?.has(item.player_id))
    .map(item => ({
      id: item.id,
      playerId: item.player_id,
      // Narrowed by the `isTableReaction` guard in the filter above, which
      // TypeScript cannot carry across a property check.
      reactionId: item.reaction_id as TableReactionID,
      targetPlayerId: item.target_player_id || undefined,
      expiresAt: item.expires_at
    })) : [];
  return {
    snapshot, version, handId: snapshot.hand_id ?? '', protocolVersion, chat, reactions
  };
}

export function applySnapshotEquity(snapshot: TableSnapshot | null, playerId: string, equity: number) {
  return snapshot ? {...snapshot, seats: snapshot.seats.map(seat =>
    seat.player_id === playerId ? {...seat, equity} : seat)} : snapshot;
}

export function revealViewerCards(snapshot: TableSnapshot | null, viewerId?: string) {
  return snapshot ? {...snapshot, seats: snapshot.seats.map(seat =>
    seat.player_id === viewerId ? {...seat, hole_cards_revealed: [true, true]} : seat)} : snapshot;
}
