'use client';
import {useState} from 'react';
import {Check, Copy, LoaderCircle, Share2, ShieldCheck} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import {createHandShare} from '@/lib/api/handShares';
import type {WalletMode} from '@/lib/api/player';

export function ShareHandDialog({handId, outcome, mode = 'sandbox'}: {
  handId: string; outcome: string; mode?: WalletMode
}) {
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<'brag' | 'bad_beat'>(outcome === 'lost' ? 'bad_beat' : 'brag');
  const [includeCards, setIncludeCards] = useState(true);
  const [expiryDays, setExpiryDays] = useState(7);
  const [pending, setPending] = useState(false);
  const [url, setURL] = useState('');
  const [copied, setCopied] = useState(false);
  
  async function create() {
    if (pending) return;
    setPending(true);
    try {
      const share = await createHandShare(handId, {
        kind, include_hero_cards: includeCards, expiry_days: expiryDays
      }, mode);
      setURL(`${window.location.origin}/share?id=${encodeURIComponent(share.token)}`);
    } finally {
      setPending(false);
    }
  }
  
  async function copy() {
    await navigator.clipboard.writeText(url);
    setCopied(true);
  }
  
  return <>
    <Button type="button" variant="outline" size="icon" onClick={() => setOpen(true)}
            aria-label="Compartilhar" title="Compartilhar esta mão">
      <Share2 aria-hidden="true"/>
    </Button>
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="share-hand-dialog">
        <DialogHeader>
          <DialogTitle>Compartilhar esta mão</DialogTitle>
          <DialogDescription>
            O link usa nomes anônimos, expira automaticamente e nunca inclui cartas que um adversário não mostrou.
          </DialogDescription>
        </DialogHeader>
        {!url ? <div className="share-hand-form">
          <fieldset>
            <legend>Como apresentar</legend>
            <label><input type="radio" name="share-kind" checked={kind === 'brag'}
                          onChange={() => setKind('brag')}/> Brag <small>Uma vitória ou jogada para
              celebrar.</small></label>
            <label><input type="radio" name="share-kind" checked={kind === 'bad_beat'}
                          onChange={() => setKind('bad_beat')}/> Bad beat <small>Uma derrota improvável para
              contar.</small></label>
          </fieldset>
          <label className="share-hand-check">
            <input type="checkbox" checked={includeCards} onChange={event => setIncludeCards(event.target.checked)}/>
            <span><b>Mostrar minhas cartas</b><small>Sem isso, somente board, resultado e ações serão publicados.</small></span>
          </label>
          <label className="share-hand-expiry">Expirar em
            <select value={expiryDays} onChange={event => setExpiryDays(Number(event.target.value))}>
              <option value={1}>24 horas</option>
              <option value={7}>7 dias</option>
              <option value={30}>30 dias</option>
            </select>
          </label>
          <p className="share-hand-privacy"><ShieldCheck aria-hidden="true"/>
            Criar o link confirma seu consentimento para publicar essa projeção anonimizada.</p>
        </div> : <div className="share-hand-created">
          <Check aria-hidden="true"/><b>Link privado criado</b>
          <p>Qualquer pessoa com este endereço poderá abrir a mão até a expiração.</p>
          <div><input readOnly value={url} aria-label="Link compartilhável"/>
            <Button type="button" onClick={copy}>{copied ? <Check/> : <Copy/>}{copied ? 'Copiado' : 'Copiar'}</Button>
          </div>
        </div>}
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Fechar</Button>
          {!url && <Button type="button" disabled={pending} onClick={create}>
            {pending ? <LoaderCircle className="spin"/> : <Share2/>} Criar link
          </Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
