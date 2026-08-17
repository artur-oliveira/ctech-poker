'use client';
import {useState} from 'react';
import Link from 'next/link';
import {
  BellOff, Flag, MoreVertical, NotebookPen, ShieldBan, ShieldCheck, UserMinus, UserPlus, UserRoundSearch, Volume2
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import type {ReportSurface, SocialRelationship} from '@/lib/api/social';
import type {SocialActionKind, SocialActionState} from '@/lib/hooks/useSocialActions';
import {ReportPlayerDialog} from '@/components/social/ReportPlayerDialog';
import {playerName} from '@/lib/utils';

export interface PlayerActionsTarget {
  player_id: string;
  name?: string;
  relationship?: SocialRelationship;
  muted?: boolean;
  blocked?: boolean;
}

// Friend affordance per current relationship. A blocked player has no
// relationship left (blocking clears it on both sides), so the friend row is
// omitted entirely rather than offered and rejected.
const FRIEND_ACTION: Record<SocialRelationship, {kind: SocialActionKind; label: string} | null> = {
  none: {kind: 'request', label: 'Adicionar amigo'},
  outgoing: {kind: 'cancel', label: 'Cancelar solicitação'},
  incoming: {kind: 'accept', label: 'Aceitar solicitação'},
  friend: {kind: 'remove', label: 'Remover amizade'}
};

/** The one player menu: seats, the public profile and every /people list use
 * this same component, so the safety actions cannot drift apart between
 * surfaces. Poker itself is never affected — nothing here touches seats,
 * stacks, actions or matchmaking. */
export function PlayerActionsMenu({target, actions, surface, tableId, handId, onEditNoteAction, onBlockedAction}: {
  target: PlayerActionsTarget;
  actions: SocialActionState;
  surface: ReportSurface;
  tableId?: string;
  handId?: string;
  onEditNoteAction?: () => void;
  // Lets the table drop the target's rendered content before the round-trip
  // (and put it back if the server rejects the block).
  onBlockedAction?: (blocked: boolean) => Promise<boolean> | boolean;
}) {
  const [open, setOpen] = useState(false);
  const [confirmingBlock, setConfirmingBlock] = useState(false);
  const [reportOpen, setReportOpen] = useState(false);
  const name = playerName(target.player_id, undefined, target.name);
  const relationship = target.relationship ?? 'none';
  const friendAction = target.blocked ? null : FRIEND_ACTION[relationship];
  const busy = actions.pending?.id === target.player_id;

  async function run(kind: SocialActionKind) {
    const ok = await actions.run(kind, target.player_id);
    if (ok) setOpen(false);
    return ok;
  }

  async function block() {
    const revert = onBlockedAction ? await onBlockedAction(true) : true;
    const ok = await run('block');
    if (!ok && revert && onBlockedAction) await onBlockedAction(false);
    setConfirmingBlock(false);
  }

  return <>
    <Popover open={open} onOpenChange={next => {
      setOpen(next);
      if (!next) setConfirmingBlock(false);
    }}>
      <PopoverTrigger render={<Button type="button" variant="ghost" size="icon"
                                     aria-label={`Ações para ${name}`}/>}>
        <MoreVertical aria-hidden="true"/>
      </PopoverTrigger>
      <PopoverContent className="social-actions-menu">
        <p className="social-actions-name">{name}</p>
        <Link href={`/profile?id=${encodeURIComponent(target.player_id)}`} className="social-actions-item">
          <UserRoundSearch aria-hidden="true"/> Ver perfil
        </Link>
        {friendAction && <button type="button" className="social-actions-item" disabled={busy}
                                onClick={() => void run(friendAction.kind)}>
          {relationship === 'friend' ? <UserMinus aria-hidden="true"/> : <UserPlus aria-hidden="true"/>}
          {friendAction.label}
        </button>}
        {relationship === 'incoming' && !target.blocked &&
          <button type="button" className="social-actions-item" disabled={busy}
                  onClick={() => void run('decline')}>
            <UserMinus aria-hidden="true"/> Recusar solicitação
          </button>}
        {onEditNoteAction && <button type="button" className="social-actions-item" onClick={() => {
          setOpen(false);
          onEditNoteAction();
        }}>
          <NotebookPen aria-hidden="true"/> Editar nota privada
        </button>}
        {target.muted
          ? <button type="button" className="social-actions-item" disabled={busy}
                    onClick={() => void run('unmute')}><Volume2 aria-hidden="true"/> Reativar chat e reações</button>
          : <button type="button" className="social-actions-item" disabled={busy}
                    onClick={() => void run('mute')}><BellOff aria-hidden="true"/> Silenciar</button>}
        {target.blocked
          ? <button type="button" className="social-actions-item" disabled={busy} onClick={() => void run('unblock')}>
            <ShieldCheck aria-hidden="true"/> Desbloquear
          </button>
          : confirmingBlock
            ? <div className="social-actions-confirm">
              <p>Bloquear esconde o chat e as reações desse jogador e desfaz a amizade. Ele continua podendo cair na
                mesma mesa pública.</p>
              <Button type="button" variant="ghost" onClick={() => setConfirmingBlock(false)}>Cancelar</Button>
              <Button type="button" disabled={busy} onClick={() => void block()}>Bloquear</Button>
            </div>
            : <button type="button" className="social-actions-item" onClick={() => setConfirmingBlock(true)}>
              <ShieldBan aria-hidden="true"/> Bloquear
            </button>}
        <button type="button" className="social-actions-item" onClick={() => {
          setOpen(false);
          setReportOpen(true);
        }}>
          <Flag aria-hidden="true"/> Denunciar
        </button>
      </PopoverContent>
    </Popover>
    <ReportPlayerDialog target={reportOpen ? target : null} surface={surface} tableId={tableId} handId={handId}
                        open={reportOpen} onOpenChangeAction={setReportOpen}/>
  </>;
}
