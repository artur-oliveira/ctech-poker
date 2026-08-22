'use client';
import {useId, useState} from 'react';
import {Check, Copy, Search, UserPlus} from 'lucide-react';
import {useQuery} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {getMe} from '@/lib/api/player';
import {lookupFriendCode, type SocialPlayer} from '@/lib/api/social';
import {socialErrorMessage} from '@/lib/social';
import type {SocialActionState} from '@/lib/hooks/useSocialActions';
import {playerName} from '@/lib/utils';

/** Discovery is exact-code only: there is no fuzzy search by display name, so
 * a name can never be used to enumerate accounts. */
export function FriendCodeLookup({actions}: { actions: SocialActionState }) {
  const inputId = useId();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  const [code, setCode] = useState('');
  const [copied, setCopied] = useState(false);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState('');
  const [found, setFound] = useState<SocialPlayer | null>(null);
  const [sent, setSent] = useState(false);

  async function copyOwnCode() {
    if (!me?.friend_code) return;
    try {
      await navigator.clipboard.writeText(me.friend_code);
      setCopied(true);
    } catch {
      // Clipboard blocked: the code stays on screen and selectable.
    }
  }

  async function search() {
    const trimmed = code.trim();
    if (!trimmed) return;
    setSearching(true);
    setError('');
    setFound(null);
    setSent(false);
    try {
      setFound(await lookupFriendCode(trimmed));
    } catch (failure) {
      setError(socialErrorMessage(failure));
    } finally {
      setSearching(false);
    }
  }

  return <section className="friend-code-shell" aria-label="Código de amizade">
    <div className="friend-code-own">
      <span>Seu código</span>
      <b>{me?.friend_code || '—'}</b>
      <Button type="button" variant="ghost" size="icon" disabled={!me?.friend_code}
              aria-label="Copiar meu código de amizade" onClick={copyOwnCode}>
        {copied ? <Check aria-hidden="true"/> : <Copy aria-hidden="true"/>}
      </Button>
    </div>
    <div className="friend-code-search">
      <Label htmlFor={inputId}>Código de um amigo</Label>
      <div className="friend-code-search-row">
        <Input id={inputId} value={code} placeholder="PKR-XXXX-XXXX-XXXX" autoComplete="off"
               onChange={event => setCode(event.target.value)}
               onKeyDown={event => event.key === 'Enter' && void search()}/>
        <Button type="button" disabled={searching || !code.trim()} onClick={() => void search()}>
          <Search aria-hidden="true"/> {searching ? 'Buscando…' : 'Buscar'}
        </Button>
      </div>
      <small>O nome de exibição não é único; só o código encontra alguém.</small>
    </div>
    {error && <p className="social-error" role="alert">{error}</p>}
    {found && <div className="friend-code-result">
        <PlayerAvatar name={found.name} avatarUrl={found.avatar_url} size={40}/>
        <b>{playerName(found.player_id, undefined, found.name)}</b>
      {sent || found.relationship === 'outgoing' ? <span role="status">Solicitação enviada</span>
        : found.relationship === 'friend' ? <span>Já é seu amigo</span>
          : <Button type="button" disabled={actions.pending?.id === found.player_id} onClick={async () => {
            if (await actions.run('request', found.player_id)) setSent(true);
          }}>
            <UserPlus aria-hidden="true"/> Adicionar amigo
          </Button>}
    </div>}
  </section>;
}
