'use client'
import {FormEvent, useState} from 'react';
import {MessageCircle, Send, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';

export function Chat({items, onSend}: { items: { player: string; message: string }[]; onSend: (s: string) => void }) {
  const [open, setOpen] = useState(false), [text, setText] = useState('');

  function submit(e: FormEvent) {
    e.preventDefault();
    if (text.trim()) {
      onSend(text.trim());
      setText('')
    }
  }

  return <aside className={`game-chat ${open ? 'open' : ''}`}>
    <Button variant="ghost" size="icon" aria-label={open ? 'Fechar chat' : 'Abrir chat'} className="chat-toggle" onClick={() => setOpen(v => !v)}>{open ? <X/> : <MessageCircle/>}</Button>
    <div className="chat-body"><h3>Mesa ao vivo</h3>
      <div className="messages">{items.map((m, i) => <p key={i}><b>{m.player.slice(0, 8)}</b>{m.message}</p>)}</div>
      <form onSubmit={submit}><Input maxLength={500} value={text} onChange={e => setText(e.target.value)}
                                     placeholder="Diga algo…"/>
        <Button size="icon" aria-label="Enviar mensagem"><Send/></Button>
      </form>
    </div>
  </aside>
}
