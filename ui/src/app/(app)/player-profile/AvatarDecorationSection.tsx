'use client';
import {type JSX, useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Ban, Check, Medal} from 'lucide-react';
import {Button, Checkbox, SkeletonList} from '@aoctech/ui';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {BADGE_ART} from '@/components/avatar/badges';
import {FRAME_ART} from '@/components/avatar/frames';
import {HAT_ART} from '@/components/avatar/hats';
import {ORNAMENT_ART} from '@/components/avatar/ornaments';
import {type AvatarSlot, avatarItemLabel, hasAvatarItems, listOwnedAvatarCosmetics} from '@/lib/avatarCosmetics';
import {type PlayerProfile, updateMe} from '@/lib/api/player';
import {PLAYER_ME_KEY} from '@/lib/hooks/useProfileEdits';
import {pushNotification} from '@/lib/notify';

export const MAX_EQUIPPED_BADGES = 3;

type RadioSlot = Exclude<AvatarSlot, 'badge'>;

const RADIO_SLOTS: Record<RadioSlot, {
  legend: string; group: string; none: string; loading: string; error: string;
  art: Record<string, () => JSX.Element>; field: 'equipped_frame_id' | 'equipped_hat_id' | 'equipped_accessory_id';
}> = {
  frame: {legend: 'Borda', group: 'Borda equipada', none: 'Nenhuma', loading: 'Carregando bordas…',
    error: 'Não foi possível carregar suas bordas agora.', art: FRAME_ART, field: 'equipped_frame_id'},
  hat: {legend: 'Chapéu', group: 'Chapéu equipado', none: 'Nenhum', loading: 'Carregando chapéus…',
    error: 'Não foi possível carregar seus chapéus agora.', art: HAT_ART, field: 'equipped_hat_id'},
  accessory: {legend: 'Adorno', group: 'Adorno equipado', none: 'Nenhum', loading: 'Carregando adornos…',
    error: 'Não foi possível carregar seus adornos agora.', art: ORNAMENT_ART, field: 'equipped_accessory_id'},
};

/** The pickers this build can show: a slot without any item has nothing to pick. */
const LIVE_RADIO_SLOTS = (Object.keys(RADIO_SLOTS) as RadioSlot[]).filter(hasAvatarItems);

const EMPTY_COPY = 'Você ainda não tem itens deste tipo. Conquiste ou compre para equipar.';

const useOwned = (slot: AvatarSlot) => useQuery({
  queryKey: ['wallet', 'avatar-cosmetic-owned', slot], queryFn: () => listOwnedAvatarCosmetics(slot),
  enabled: hasAvatarItems(slot),
});

/** Loading / error-with-retry / empty around a slot's owned list. `saved`
 * ids no longer owned (revoked, expired) are listed too, so the picker always
 * shows what the profile wears. */
function OwnedList({query, saved, loading, error, children}: {
  query: ReturnType<typeof useOwned>; saved: string[]; loading: string; error: string;
  children: (ids: string[]) => JSX.Element;
}) {
  if (query.isLoading) return <SkeletonList label={loading} count={1} height={44} className="skeleton-panel"/>;
  if (query.isError) return <div className="lobby-empty">{error}
    <Button variant="outline" size="sm" onClick={() => void query.refetch()}>Tentar novamente</Button>
  </div>;
  const owned = query.data ?? [];
  const ids = [...owned, ...saved.filter(id => id && !owned.includes(id))];
  return ids.length === 0 ? <p className="player-profile-empty">{EMPTY_COPY}</p> : children(ids);
}

/**
 * Native radios (arrow keys for free) dressed as chips. The check mark, not
 * the tint, says which one is on.
 */
function SlotOption({name, checked, label, onSelect, children}: {
  name: string; checked: boolean; label: string; onSelect: () => void; children?: JSX.Element;
}) {
  return <label className="avatar-decoration-option">
    <input type="radio" name={name} checked={checked} onChange={onSelect}/>
    <span className="avatar-decoration-option-face">
      <span className="avatar-decoration-swatch" aria-hidden="true">{children}</span>
      {label}
      {checked && <Check className="avatar-decoration-check" aria-hidden="true"/>}
    </span>
  </label>;
}

/**
 * Avatar decoration (#292): one owned frame, hat and ornament (accessory
 * slot) and up to three owned badges, saved through the same `POST /players/me` every other profile
 * field uses. Only owned ids are listed (granted or bought elsewhere; nothing
 * locked to browse here). Both previews render the pending, unsaved choice.
 */
export function AvatarDecorationSection({me}: { me: PlayerProfile }) {
  const queryClient = useQueryClient();
  const saved = {
    frame: me.equipped_frame_id ?? '', hat: me.equipped_hat_id ?? '', accessory: me.equipped_accessory_id ?? '',
  };
  const [picked, setPicked] = useState<Record<RadioSlot, string>>(saved);
  const [badgeIds, setBadgeIds] = useState<string[]>(me.equipped_badge_ids ?? []);

  const owned = {frame: useOwned('frame'), hat: useOwned('hat'), accessory: useOwned('accessory')};
  const ownedBadges = useOwned('badge');

  const savedBadgeIds = me.equipped_badge_ids ?? [];
  const badgesChanged = badgeIds.length !== savedBadgeIds.length || badgeIds.some(id => !savedBadgeIds.includes(id));
  const changes: Parameters<typeof updateMe>[0] = {};
  for (const slot of LIVE_RADIO_SLOTS) {
    if (picked[slot] !== saved[slot]) changes[RADIO_SLOTS[slot].field] = picked[slot];
  }
  if (badgesChanged) changes.equipped_badge_ids = badgeIds;
  const changed = Object.keys(changes).length > 0;

  const save = useMutation({
    mutationFn: () => updateMe(changes),
    onSuccess: (profile: PlayerProfile) => {
      queryClient.setQueryData(PLAYER_ME_KEY, profile);
      pushNotification('Visual do avatar atualizado.', 'info');
    },
    // Fields apply one by one server-side: a late rejection can leave earlier
    // slots saved, so resync with what the server actually holds.
    onError: () => {
      void queryClient.invalidateQueries({queryKey: PLAYER_ME_KEY});
      pushNotification('Não foi possível salvar agora. Tente novamente.', 'error');
    }
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

  const preview = {
    name: me.name, avatarUrl: me.avatar_url, badgeIds,
    frameId: picked.frame || undefined, hatId: picked.hat || undefined, accessoryId: picked.accessory || undefined,
  };

  return <section className="player-profile-section" aria-labelledby="player-profile-avatar-decoration-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-avatar-decoration-title">Visual do avatar</h2>
      <p>Borda, chapéu, adorno e emblemas aparecem ao redor da sua foto em qualquer mesa e na sua vitrine.</p>
    </header>

    <div className="avatar-decoration-preview" aria-hidden="true">
      <figure><PlayerAvatar {...preview} size={72}/><figcaption>Perfil</figcaption></figure>
      <figure><PlayerAvatar {...preview} size={36}/><figcaption>Na mesa</figcaption></figure>
    </div>

    {LIVE_RADIO_SLOTS.map(slot => {
      const cfg = RADIO_SLOTS[slot];
      const name = `avatar-decoration-${slot}`;
      return <fieldset key={slot} className="avatar-decoration-group">
        <legend>{cfg.legend}</legend>
        <OwnedList query={owned[slot]} saved={[saved[slot]]} loading={cfg.loading} error={cfg.error}>
          {ids => <div className="avatar-decoration-options" role="radiogroup" aria-label={cfg.group}>
            <SlotOption name={name} checked={picked[slot] === ''} label={cfg.none}
                        onSelect={() => setPicked(p => ({...p, [slot]: ''}))}><Ban/></SlotOption>
            {ids.map(id => {
              const Art = cfg.art[id];
              return <SlotOption key={id} name={name} checked={picked[slot] === id} label={avatarItemLabel(slot, id)}
                                 onSelect={() => setPicked(p => ({...p, [slot]: id}))}>
                {Art && <Art/>}
              </SlotOption>;
            })}
          </div>}
        </OwnedList>
      </fieldset>;
    })}

    <fieldset className="avatar-decoration-group">
      <legend>Emblemas <span className="avatar-decoration-badge-count">{badgeIds.length}/{MAX_EQUIPPED_BADGES}</span></legend>
      <OwnedList query={ownedBadges} saved={savedBadgeIds} loading="Carregando emblemas…" error="Não foi possível carregar seus emblemas agora.">
        {ids => <ul className="avatar-decoration-badges-list">
          {ids.map(id => {
            const Glyph = BADGE_ART[id] ?? Medal;
            return <li key={id}>
              <label className="avatar-decoration-badge-option">
                <Checkbox checked={badgeIds.includes(id)} onCheckedChange={() => toggleBadge(id)}/>
                <Glyph aria-hidden="true"/> {avatarItemLabel('badge', id)}
              </label>
            </li>;
          })}
        </ul>}
      </OwnedList>
    </fieldset>

    <div className="player-profile-actions">
      <Button type="button" disabled={!changed || save.isPending} loading={save.isPending} onClick={() => save.mutate()}>
        {save.isPending ? 'Salvando…' : 'Salvar visual do avatar'}
      </Button>
    </div>
  </section>;
}
