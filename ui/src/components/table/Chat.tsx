'use client';
import {SyntheticEvent, useEffect, useId, useRef, useState} from 'react';
import {MessageCircle, Send, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {SeatView} from '@/lib/api/table';
import {playerName} from '@/lib/utils';
import {useDismiss} from '@/lib/hooks/useDismiss';
import {useHoverPanel} from '@/lib/hooks/useHoverPanel';
import {CHAT_MESSAGE_MAX_LENGTH} from '@/lib/chat';

type ChatItem = { id: string; player: string; message: string; timestamp?: number };

export function Chat({items, onSendAction, connected = true, viewerId, seats = [], open, onOpenChangeAction}: {
  items: ChatItem[];
  onSendAction: (message: string) => boolean;
  connected?: boolean;
  viewerId?: string;
  seats?: SeatView[];
  open: boolean;
  onOpenChangeAction: (open: boolean) => void;
}) {
  const [text, setText] = useState('');
  const [sendError, setSendError] = useState('');
  const panelId = useId();
  const inputId = useId();
  const errorId = useId();
  const charCountId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const asideRef = useRef<HTMLElement>(null);
  const latest = items.at(-1);
  const nameOf = (id: string) => playerName(id, viewerId, seats.find(seat => seat.player_id === id)?.name);
  // Opening the drawer is the acknowledgement boundary, but `open` is owned by
  // the parent — there is no local close event to hook. So the count is
  // adjusted during render (React's documented answer for "state that follows
  // a prop") rather than in an effect: the extra render is discarded before
  // the browser paints, where an effect would flash a stale unread badge.
  const [seenCount, setSeenCount] = useState(items.length);
  if (open && seenCount !== items.length) setSeenCount(items.length);
  const unread = open ? 0 : Math.max(0, items.length - seenCount);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  useEffect(() => {
    const node = messagesRef.current;
    if (node) node.scrollTop = node.scrollHeight;
  }, [items.length, open]);

  function submit(event: SyntheticEvent) {
    event.preventDefault();
    const message = text.trim();
    if (!message) return;
    if (!connected || !onSendAction(message)) {
      setSendError('Mensagem não enviada. Reconecte à mesa e tente novamente.');
      return;
    }
    setText('');
    setSendError('');
  }

  useDismiss(asideRef, open, () => onOpenChangeAction(false));
  const hover = useHoverPanel(onOpenChangeAction);

  return <aside ref={asideRef} className={`game-chat table-aside-skirt ${open ? 'open' : ''}`}
                aria-label="Chat da mesa" {...hover}>
    <div className="sr-only" role="status" aria-live={open ? 'off' : 'polite'} aria-atomic="true">
      {latest ? `${nameOf(latest.player)} disse: ${latest.message}` : ''}
    </div>
    <Button type="button" variant="ghost" size="icon" aria-label={open ? 'Fechar chat' : 'Abrir chat'}
            aria-expanded={open} aria-controls={panelId} aria-keyshortcuts="t" className="chat-toggle"
            onClick={() => onOpenChangeAction(!open)}>
      {open ? <X/> : <MessageCircle/>}
      {unread > 0 && <span className="chat-unread-dot" aria-hidden="true"/>}
    </Button>
    <div id={panelId} className="chat-body" aria-hidden={!open}>
      <div className="chat-panel-header">
        <h2>Chat da mesa</h2>
        <Button type="button" variant="ghost" size="icon" aria-label="Fechar painel de chat"
                onClick={() => onOpenChangeAction(false)}><X/></Button>
      </div>
      <div className="messages" role="log" aria-live="polite" aria-relevant="additions text" ref={messagesRef}>
        {items.length === 0 ? <p className="messages-empty">Nenhuma mensagem ainda. Diga um oi para a mesa.</p> :
          items.map(message => <p key={message.id}>
            <b>{nameOf(message.player)}</b>{message.message}
          </p>)}
      </div>
      <form onSubmit={submit}>
        <label className="sr-only" htmlFor={inputId}>Mensagem para a mesa</label>
        <Input id={inputId} ref={inputRef} maxLength={CHAT_MESSAGE_MAX_LENGTH} value={text} disabled={!connected}
               onChange={event => {
                 setText(event.target.value);
                 if (sendError) setSendError('');
               }} placeholder={connected ? 'Diga algo…' : 'Reconectando…'} aria-invalid={Boolean(sendError)}
               aria-describedby={sendError ? errorId : charCountId}/>
        <span id={charCountId} className="chat-char-count" aria-live="polite">
          {text.length}/{CHAT_MESSAGE_MAX_LENGTH}
        </span>
        <Button type="submit" size="icon" aria-label="Enviar mensagem"
                disabled={!text.trim() || !connected}><Send/></Button>
      </form>
      {sendError && <p id={errorId} className="chat-error" role="alert">{sendError}</p>}
    </div>
  </aside>;
}
