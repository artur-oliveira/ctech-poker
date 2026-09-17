'use client';
import {useRef, useState} from 'react';
import {useMutation, useQueryClient} from '@tanstack/react-query';
import {Camera, LoaderCircle, Trash2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Field} from '@/components/ui/field';
import {Input} from '@/components/ui/input';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {deleteAvatar, uploadAvatar} from '@/lib/avatar';
import {type PlayerProfile, updateMe} from '@/lib/api/player';
import {pushNotification} from '@/lib/notify';

const NAME_MAX_LENGTH = 40;

/** Name and photo: the two things every other player sees first. */
export function IdentitySection({me}: {me: PlayerProfile}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(me.name ?? '');
  const fileInput = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: profile => {
      queryClient.setQueryData(['player', 'me'], profile);
      pushNotification(`Agora você joga como ${profile.name}.`, 'info');
    }
  });
  const avatar = useMutation({
    mutationFn: uploadAvatar,
    onSuccess: profile => {
      queryClient.setQueryData(['player', 'me'], profile);
      pushNotification('Foto de perfil atualizada.', 'info');
    },
    onError: () => pushNotification('Não foi possível atualizar a foto. Tente outra imagem.')
  });
  const removeAvatar = useMutation({
    mutationFn: deleteAvatar,
    onSuccess: profile => {
      queryClient.setQueryData(['player', 'me'], profile);
      pushNotification('Foto de perfil removida.', 'info');
    }
  });

  const trimmed = name.trim();
  const changed = trimmed.length > 0 && trimmed !== (me.name ?? '');

  return <section className="player-profile-section" aria-labelledby="player-profile-identity-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-identity-title">Identidade</h2>
      <p>Nome e foto aparecem na mesa, no ranking e na sua vitrine.</p>
    </header>
    <div className="player-profile-identity">
      <div className="player-profile-avatar">
        <PlayerAvatar name={me.name} avatarUrl={me.avatar_url} size={96}/>
        <Button type="button" size="icon" className="profile-avatar-camera" disabled={avatar.isPending}
                aria-label={me.avatar_url ? 'Trocar foto de perfil' : 'Adicionar foto de perfil'}
                onClick={() => fileInput.current?.click()}>
          {avatar.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Camera aria-hidden="true"/>}
        </Button>
        <input ref={fileInput} className="sr-only" type="file" accept="image/jpeg,image/png"
               aria-label="Selecionar foto de perfil" onChange={event => {
          const file = event.target.files?.[0];
          if (file) avatar.mutate(file);
          event.target.value = '';
        }}/>
      </div>
      <form className="player-profile-name" onSubmit={event => {
        event.preventDefault();
        if (changed) save.mutate({name: trimmed});
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
          {me.avatar_url && <Button type="button" variant="ghost" loading={removeAvatar.isPending}
                                    onClick={() => removeAvatar.mutate()}>
            {!removeAvatar.isPending && <Trash2 aria-hidden="true"/>} Remover foto
          </Button>}
        </div>
      </form>
    </div>
  </section>;
}
