'use client';
import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Ban, Frame as FrameIcon, Medal} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {SkeletonList} from '@/components/ui/skeleton';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {avatarBadgeLabel, avatarFrameLabel, listOwnedAvatarCosmetics} from '@/lib/avatarCosmetics';
import {type PlayerProfile, updateMe} from '@/lib/api/player';
import {PLAYER_ME_KEY} from '@/lib/hooks/useProfileEdits';
import {pushNotification} from '@/lib/notify';

export const MAX_EQUIPPED_BADGES = 3;

/**
 * Seasonal avatar decoration (#292): pick one owned frame and up to three
 * owned badges, saved together through the same `POST /players/me` every
 * other profile field already uses. Only owned ids are ever listed — there
 * is no wallet catalog for these (they are granted, not sold), so there is
 * nothing locked to browse here, unlike DeckPicker's premium items.
 */
export function AvatarDecorationSection({me}: { me: PlayerProfile }) {
  const queryClient = useQueryClient();
  const [frameId, setFrameId] = useState(me.equipped_frame_id ?? '');
  const [badgeIds, setBadgeIds] = useState<string[]>(me.equipped_badge_ids ?? []);

  const ownedFrames = useQuery({
    queryKey: ['wallet', 'avatar-cosmetic-owned', 'frame'], queryFn: () => listOwnedAvatarCosmetics('frame')
  });
  const ownedBadges = useQuery({
    queryKey: ['wallet', 'avatar-cosmetic-owned', 'badge'], queryFn: () => listOwnedAvatarCosmetics('badge')
  });

  const save = useMutation({
    mutationFn: () => updateMe({equipped_frame_id: frameId, equipped_badge_ids: badgeIds}),
    onSuccess: (profile: PlayerProfile) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      pushNotification('Moldura e emblemas atualizados.', 'info');
    },
    onError: () => pushNotification('Não foi possível salvar agora. Tente novamente.', 'error')
  });

  function toggleBadge(id: string) {
    setBadgeIds(current => {
      if (current.includes(id)) return current.filter(item => item !== id);
      if (current.length >= MAX_EQUIPPED_BADGES) {
        pushNotification(`Escolha no máximo ${MAX_EQUIPPED_BADGES} emblemas.`, 'info');
        return current;
      }
      return [...current, id];
    });
  }

  const frames = ownedFrames.data ?? [];
  const badges = ownedBadges.data ?? [];
  const savedBadgeIds = me.equipped_badge_ids ?? [];
  const changed = frameId !== (me.equipped_frame_id ?? '') ||
    badgeIds.length !== savedBadgeIds.length || badgeIds.some(id => !savedBadgeIds.includes(id));

  return <section className="player-profile-section" aria-labelledby="player-profile-avatar-decoration-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-avatar-decoration-title">Moldura e emblemas</h2>
      <p>Desbloqueados em temporadas. Aparecem ao redor da sua foto em qualquer mesa e na sua vitrine.</p>
    </header>

    <div className="avatar-decoration-preview" aria-hidden="true">
      <PlayerAvatar name={me.name} avatarUrl={me.avatar_url} size={72}
                    frameId={frameId || undefined} badgeIds={badgeIds}/>
    </div>

    <fieldset className="avatar-decoration-group">
      <legend>Moldura</legend>
      {ownedFrames.isLoading
        ? <SkeletonList label="Carregando molduras…" count={1} height={44} className="skeleton-panel"/>
        : ownedFrames.isError
          ? <div className="lobby-empty">Não foi possível carregar suas molduras agora.
            <Button variant="outline" size="sm" onClick={() => void ownedFrames.refetch()}>Tentar novamente</Button>
          </div>
          : frames.length === 0
            ? <p className="player-profile-empty">Nenhuma moldura desbloqueada ainda. Molduras sazonais aparecem
              aqui quando você conquista uma.</p>
            : <div className="avatar-decoration-options" role="radiogroup" aria-label="Moldura equipada">
              <button type="button" role="radio" aria-checked={frameId === ''} className="avatar-decoration-option"
                      onClick={() => setFrameId('')}>
                <Ban aria-hidden="true"/> Nenhuma
              </button>
              {frames.map(id => <button key={id} type="button" role="radio" aria-checked={frameId === id}
                                        className="avatar-decoration-option" onClick={() => setFrameId(id)}>
                <FrameIcon aria-hidden="true"/> {avatarFrameLabel(id)}
              </button>)}
            </div>}
    </fieldset>

    <fieldset className="avatar-decoration-group">
      <legend>Emblemas <span className="avatar-decoration-badge-count">{badgeIds.length}/{MAX_EQUIPPED_BADGES}</span></legend>
      {ownedBadges.isLoading
        ? <SkeletonList label="Carregando emblemas…" count={1} height={44} className="skeleton-panel"/>
        : ownedBadges.isError
          ? <div className="lobby-empty">Não foi possível carregar seus emblemas agora.
            <Button variant="outline" size="sm" onClick={() => void ownedBadges.refetch()}>Tentar novamente</Button>
          </div>
          : badges.length === 0
            ? <p className="player-profile-empty">Nenhum emblema desbloqueado ainda. Emblemas sazonais aparecem
              aqui quando você conquista um.</p>
            : <ul className="avatar-decoration-badges-list">
              {badges.map(id => <li key={id}>
                <label className="avatar-decoration-badge-option">
                  <Checkbox checked={badgeIds.includes(id)} onCheckedChange={() => toggleBadge(id)}/>
                  <Medal aria-hidden="true"/> {avatarBadgeLabel(id)}
                </label>
              </li>)}
            </ul>}
    </fieldset>

    <div className="player-profile-actions">
      <Button type="button" disabled={!changed} loading={save.isPending} onClick={() => save.mutate()}>
        {save.isPending ? 'Salvando…' : 'Salvar moldura e emblemas'}
      </Button>
    </div>
  </section>;
}
