'use client';
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {Field} from '@/components/ui/field';
import {Input} from '@/components/ui/input';
import {ProfilePhotoEditor, ProfilePhotoRemoveButton} from '@/components/profile/ProfilePhoto';
import {type PlayerProfile} from '@/lib/api/player';
import {useProfileNameSave} from '@/lib/hooks/useProfileEdits';

const NAME_MAX_LENGTH = 40;

/**
 * Name and photo: the two things every other player sees first. The header
 * popover edits the same two fields in its own compact shape; what is shared
 * is the write, not the layout (`useProfileNameSave`, `ProfilePhotoEditor`),
 * so neither surface can save differently from the other.
 */
export function IdentitySection({me}: {me: PlayerProfile}) {
  const [name, setName] = useState(me.name ?? '');
  const save = useProfileNameSave();

  const trimmed = name.trim();
  const changed = trimmed.length > 0 && trimmed !== (me.name ?? '');

  return <section className="player-profile-section" aria-labelledby="player-profile-identity-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-identity-title">Identidade</h2>
      <p>Nome e foto aparecem na mesa, no ranking e na sua vitrine.</p>
    </header>
    <div className="player-profile-identity">
      <ProfilePhotoEditor className="player-profile-avatar" name={me.name} avatarUrl={me.avatar_url} size={96}/>
      <form className="player-profile-name" onSubmit={event => {
        event.preventDefault();
        if (changed) save.saveName(name);
      }}>
        <Field label="Nome de exibição" description={`Até ${NAME_MAX_LENGTH} caracteres.`}>
          {control => <Input {...control} value={name} maxLength={NAME_MAX_LENGTH}
                             placeholder="Como quer ser chamado na mesa"
                             onChange={event => setName(event.target.value)}/>}
        </Field>
        <div className="player-profile-actions">
          <Button type="submit" disabled={!changed} loading={save.isPending}>
            {save.isPending ? 'Salvando…' : 'Salvar nome'}
          </Button>
          {me.avatar_url && <ProfilePhotoRemoveButton>Remover foto</ProfilePhotoRemoveButton>}
        </div>
      </form>
    </div>
  </section>;
}
