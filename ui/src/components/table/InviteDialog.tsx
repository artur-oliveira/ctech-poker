'use client';
import {useId, useState} from 'react';
import {Check, Copy, Share2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';
import {PeopleList} from '@/components/social/PeopleList';
import {listFriends, sendTableInvite, type SocialPlayer} from '@/lib/api/social';
import {useSocialList} from '@/lib/hooks/useSocialList';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {SOCIAL_KEYS, socialErrorMessage} from '@/lib/social';
import {playerName} from '@/lib/utils';

/** Link sharing stays exactly as it was; `roomId` adds the friends section,
 * which sends in-app invites instead. The invite never carries the room's
 * share code — accepting it creates a server-side grant instead. */
export function InviteDialog({url, roomId}: { url: string; roomId?: string }) {
  const searchId = useId();
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [search, setSearch] = useState('');
  const [invited, setInvited] = useState<string[]>([]);
  const [failed, setFailed] = useState<Record<string, string>>({});
  const actions = useSocialActions();
  const friends = useSocialList(SOCIAL_KEYS.friends, listFriends, Boolean(roomId) && open);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable/blocked. The input stays visible and selectable for a manual copy.
    }
  }

  async function share() {
    if (navigator.share) {
      try {
        await navigator.share({url});
      } catch {
        // User dismissed the native share sheet, nothing to recover from.
      }
      return;
    }
    await copy();
  }

  // The dialog deliberately stays open: inviting several friends is one task,
  // and each row keeps its own result.
  async function invite(friend: SocialPlayer) {
    if (!roomId) return;
    setFailed(previous => Object.fromEntries(
      Object.entries(previous).filter(([playerId]) => playerId !== friend.player_id)));
    try {
      await sendTableInvite(friend.player_id, roomId);
      setInvited(previous => [...previous, friend.player_id]);
    } catch (failure) {
      setFailed(previous => ({...previous, [friend.player_id]: socialErrorMessage(failure)}));
    }
  }

  const term = search.trim().toLocaleLowerCase('pt-BR');
  const matches = friends.items.filter(friend => !term ||
    playerName(friend.player_id, undefined, friend.name).toLocaleLowerCase('pt-BR').includes(term));

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Convidar para a mesa"/>}>
      <Share2/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Convidar para a mesa</DialogTitle>
        <DialogDescription>Compartilhe este link para chamar alguém para esta mesa.</DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        <Input readOnly value={url} aria-label="Link de convite" onFocus={event => event.currentTarget.select()}/>
        <Button type="button" onClick={share}>
          {copied ? <><Check/> Copiado</> : <><Copy/> Copiar</>}
        </Button>
      </div>
      {roomId && <section className="invite-friends" aria-labelledby="invite-friends-title">
        <h3 id="invite-friends-title">Amigos</h3>
        <Label htmlFor={searchId}>Buscar na sua lista</Label>
        <Input id={searchId} value={search} placeholder="Nome do amigo" autoComplete="off"
               onChange={event => setSearch(event.target.value)}/>
        <PeopleList variant="friends" items={matches} isLoading={friends.isLoading} isError={friends.isError}
                    onRetryAction={friends.retry} hasNext={friends.hasNext} loadingMore={friends.loadingMore}
                    onMoreAction={friends.loadMore} actions={actions} invitedIds={invited}
                    onInviteAction={friend => void invite(friend)}
                    emptyTitle={term ? 'Nenhum amigo com esse nome.' : 'Você ainda não tem amigos para convidar.'}
                    emptyHint={term ? undefined : 'Use seu código de amizade em Pessoas para adicionar alguém.'}/>
        {Object.entries(failed).map(([playerId, message]) =>
          <p key={playerId} className="social-error" role="alert">{message}</p>)}
      </section>}
    </DialogContent>
  </Dialog>;
}
