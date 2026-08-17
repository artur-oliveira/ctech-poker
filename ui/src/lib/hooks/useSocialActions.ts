'use client';
import {useCallback, useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {
  acceptFriendRequest, acceptTableInvite, blockPlayer, cancelFriendRequest, declineFriendRequest, declineTableInvite,
  mutePlayer, removeFriend, sendFriendRequest, unblockPlayer, unmutePlayer
} from '@/lib/api/social';
import {pushNotification} from '@/lib/notify';
import {SOCIAL_KEYS, socialErrorMessage} from '@/lib/social';

export type SocialActionKind =
  'request' | 'accept' | 'decline' | 'cancel' | 'remove' | 'mute' | 'unmute' | 'block' | 'unblock'
  | 'accept-invite' | 'decline-invite';

// Every social mutation is "send it, then re-read the authoritative list":
// relationships are mirrored server-side in one transaction, so a locally
// patched cache would be a second source of truth for the same edge.
const RUNNERS: Record<SocialActionKind, (id: string) => Promise<unknown>> = {
  request: playerId => sendFriendRequest({player_id: playerId}),
  accept: acceptFriendRequest,
  decline: declineFriendRequest,
  cancel: cancelFriendRequest,
  remove: removeFriend,
  mute: mutePlayer,
  unmute: unmutePlayer,
  block: blockPlayer,
  unblock: unblockPlayer,
  'accept-invite': acceptTableInvite,
  'decline-invite': declineTableInvite
};

export interface SocialActionState {
  run: (kind: SocialActionKind, id: string) => Promise<boolean>;
  pending: {kind: SocialActionKind; id: string} | null;
}

/** Shared action core for the drawer, the /people lists, the public profile
 * and the seat menu. `id` is a player id for relationship actions and an event
 * id for the two invite actions. */
export function useSocialActions(): SocialActionState {
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<{kind: SocialActionKind; id: string} | null>(null);

  const run = useCallback(async (kind: SocialActionKind, id: string) => {
    setPending({kind, id});
    try {
      await RUNNERS[kind](id);
      await queryClient.invalidateQueries({queryKey: SOCIAL_KEYS.root});
      return true;
    } catch (failure) {
      // One toast, from one place: every surface that runs a social action
      // (drawer, page, seat menu) reports failures the same way.
      pushNotification(socialErrorMessage(failure));
      return false;
    } finally {
      setPending(null);
    }
  }, [queryClient]);

  return {run, pending};
}
