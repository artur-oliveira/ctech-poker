'use client';
import {useMutation, useQueryClient} from '@tanstack/react-query';
import {type PlayerProfile, updateMe} from '@/lib/api/player';
import {deleteAvatar, uploadAvatar} from '@/lib/avatar';
import {pushNotification} from '@/lib/notify';

/**
 * Name, photo and deck have two editors on purpose: the header popover is the
 * quick path a player opens many times a session, and `/player-profile` is the
 * full sheet that also holds the showcase. Two surfaces, but never two
 * implementations: every write to the profile row goes through the mutations
 * here, so the request, the cache write and the confirmation copy exist once.
 *
 * Coherence between the surfaces falls out of that. Each mutation replaces
 * `['player','me']` with the server's answer, and both surfaces read that same
 * query, so saving the name in the popover repaints the route's field (and the
 * other way round) with no local copy left behind. Neither surface keeps a
 * mirrored profile: the only local state is the draft being typed.
 */
export const PLAYER_ME_KEY = ['player', 'me'];

/** The display name. Trimming and the empty guard live here so a blank save
 * cannot reach the server from either surface. */
export function useProfileNameSave() {
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: (profile: PlayerProfile) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      pushNotification(`Agora você joga como ${profile.name}.`, 'info');
    }
  });
  return {
    isPending: save.isPending,
    /** Returns whether the save was actually submitted, so a caller can keep
     * its editor open on an empty draft instead of closing on nothing. */
    saveName(draft: string) {
      const name = draft.trim();
      if (!name) return false;
      save.mutate({name});
      return true;
    }
  };
}

export function useAvatarUpload() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: uploadAvatar,
    onSuccess: (profile: PlayerProfile) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      pushNotification('Foto de perfil atualizada.', 'info');
    },
    onError: () => pushNotification('Não foi possível atualizar a foto. Tente outra imagem.')
  });
}

export function useAvatarRemove() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteAvatar,
    onSuccess: (profile: PlayerProfile) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      pushNotification('Foto de perfil removida.', 'info');
    }
  });
}

/** Deck variant and, where the real-money UI is on, wallet mode: the two
 * single-field preferences that take effect on the next hand. */
export function useTablePreferenceSave() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateMe,
    onSuccess: (profile: PlayerProfile, input) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      if (input?.deck_variant) pushNotification('Baralho pronto para a próxima mão.', 'info');
      if (input?.wallet_mode) pushNotification(
        input.wallet_mode === 'real' ? 'Modo dinheiro real selecionado.' : 'Modo fichas selecionado.', 'info'
      );
    },
    onError: (_error, input) => {
      // A rejected wallet-mode change leaves the profile untouched; re-sync
      // from the server so the Switch snaps back to the real mode and tell the
      // player nothing changed (the generic API toast doesn't say which mode).
      if (input?.wallet_mode) {
        void queryClient.invalidateQueries({queryKey: PLAYER_ME_KEY});
        pushNotification('Não foi possível trocar o modo de jogo. Seu modo atual foi mantido.');
      }
    }
  });
}
