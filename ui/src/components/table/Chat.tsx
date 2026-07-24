'use client';
import {FormEvent, useEffect, useId, useRef, useState} from 'react';
import {MessageCircle, Send, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import type {SeatView} from '@/lib/api/table';
import {playerName} from '@/lib/utils';

type ChatItem = { player: string; message: string };

export function Chat({items, onSend, connected = true, viewerId, seats = []}: {
  items: ChatItem[];
  onSend: (message: string) => boolean;
  connected?: boolean;
  viewerId?: string;
  seats?: SeatView[]
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');
  const [sendError, setSendError] = useState('');
  const panelId = useId();
  const inputId = useId();
  const errorId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const latest = items.at(-1);
  const nameOf = (id: string) => playerName(id, viewerId, seats.find(seat => seat.player_id === id)?.name);
  const [seenCount, setSeenCount] = useState(items.length);
  if (open && items.length !== seenCount) setSeenCount(items.length);
  const unread = open ? 0 : Math.max(0, items.length - seenCount);

  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  useEffect(() => {
    const node = messagesRef.current;
    if (node) node.scrollTop = node.scrollHeight;
  }, [items.length, open]);

  function submit(event: FormEvent) {
    event.preventDefault();
    const message = text.trim();
    if (!message) return;
    if (!connected || !onSend(message)) {
      setSendError('Mensagem não enviada. Reconecte à mesa e tente novamente.');
      return;
    }
    setText('');
    setSendError('');
  }

  return <aside className={`game-chat ${open ? 'open' : ''}`} aria-label="Chat da mesa">
    <div className="sr-only" role="status" aria-live={open ? 'off' : 'polite'} aria-atomic="true">
      {latest ? `${nameOf(latest.player)} disse: ${latest.message}` : ''}
    </div>
    <Button type="button" variant="ghost" size="icon" aria-label={open ? 'Fechar chat' : 'Abrir chat'}
            aria-expanded={open} aria-controls={panelId} className="chat-toggle"
            onClick={() => setOpen(value => !value)}>
      {open ? <X/> : <MessageCircle/>}
      {unread > 0 && <span className="chat-unread-dot" aria-hidden="true"/>}
    </Button>
    <div id={panelId} className="chat-body" aria-hidden={!open}>
      <h3>Chat da mesa</h3>
      <div className="messages" role="log" aria-live="polite" aria-relevant="additions text" ref={messagesRef}>
        {items.length === 0 ? <p className="messages-empty">Nenhuma mensagem ainda. Diga um oi para a mesa.</p> :
          items.map((message, index) => <p key={`${index}-${message.player}-${message.message}`}>
            <b>{nameOf(message.player)}</b>{message.message}
          </p>)}
      </div>
      <form onSubmit={submit}>
        <label className="sr-only" htmlFor={inputId}>Mensagem para a mesa</label>
        <Input id={inputId} ref={inputRef} maxLength={500} value={text} disabled={!connected}
               onChange={event => {
                 setText(event.target.value);
                 if (sendError) setSendError('');
               }} placeholder={connected ? 'Diga algo…' : 'Reconectando…'} aria-invalid={Boolean(sendError)}
               aria-describedby={sendError ? errorId : undefined}/>
        <Button type="submit" size="icon" aria-label="Enviar mensagem"
                disabled={!text.trim() || !connected}><Send/></Button>
      </form>
      {sendError && <p id={errorId} className="chat-error" role="alert">{sendError}</p>}
    </div>
  </aside>;
}
