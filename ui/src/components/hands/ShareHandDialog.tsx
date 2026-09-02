'use client';
import {useState} from 'react';
import {
  Check, CircleAlert, Clock3, Copy, HeartCrack, LoaderCircle, Share2, ShieldCheck, ShieldOff, Trophy
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import {createHandShare, revokeHandShare} from '@/lib/api/handShares';
import type {WalletMode} from '@/lib/api/player';
import {clearPersistedHandShare, getPersistedHandShare, setPersistedHandShare} from '@/lib/handShareStorage';

function shareURL(token: string) {
  return `${window.location.origin}/share?id=${encodeURIComponent(token)}`;
}

export function ShareHandDialog({handId, outcome, mode = 'sandbox'}: {
  handId: string; outcome: string; mode?: WalletMode
}) {
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<'brag' | 'bad_beat'>(outcome === 'lost' ? 'bad_beat' : 'brag');
  const [includeCards, setIncludeCards] = useState(true);
  const [expiryDays, setExpiryDays] = useState(7);
  const [pending, setPending] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [revoked, setRevoked] = useState(false);
  const existing = getPersistedHandShare(handId);
  const [token, setToken] = useState(existing?.token ?? '');
  const [url, setURL] = useState(existing ? shareURL(existing.token) : '');
  const [justCreated, setJustCreated] = useState(false);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState('');

  async function create() {
    if (pending) return;
    setPending(true);
    setError('');
    try {
      const share = await createHandShare(handId, {
        kind, include_hero_cards: includeCards, expiry_days: expiryDays
      }, mode);
      setToken(share.token);
      setURL(shareURL(share.token));
      setJustCreated(true);
      setRevoked(false);
      setPersistedHandShare(handId, {token: share.token, expiresAt: share.expires_at});
    } catch {
      setError('Não foi possível criar o link agora. Tente novamente em instantes.');
    } finally {
      setPending(false);
    }
  }

  async function copy() {
    setError('');
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      setCopied(false);
      setError('Não foi possível copiar automaticamente. Selecione o link e copie manualmente.');
    }
  }

  async function revoke() {
    if (revoking) return;
    setRevoking(true);
    setError('');
    try {
      await revokeHandShare(token);
      clearPersistedHandShare(handId);
      setURL('');
      setToken('');
      setCopied(false);
      setJustCreated(false);
      setRevoked(true);
    } catch {
      setError('Não foi possível revogar o link agora. Tente novamente em instantes.');
    } finally {
      setRevoking(false);
    }
  }

  return <>
    <Button type="button" variant="outline" size="icon" onClick={() => setOpen(true)}
            aria-label="Compartilhar" title="Compartilhar esta mão">
      <Share2 aria-hidden="true"/>
    </Button>
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="share-hand-dialog">
        <DialogHeader className="share-hand-header">
          <span className="share-hand-mark" aria-hidden="true"><Share2/></span>
          <DialogTitle>Compartilhar esta mão</DialogTitle>
          <DialogDescription>
            Conte a jogada do seu jeito. Nomes ficam anônimos e cartas ocultas continuam ocultas.
          </DialogDescription>
        </DialogHeader>
        {revoked && <p className="share-hand-revoked" role="status">
          <ShieldOff aria-hidden="true"/>
          <span><b>Link revogado.</b> Quem tiver o endereço vai ver que ele expirou ou foi revogado.</span>
        </p>}
        {!url ? <div className="share-hand-form">
          <fieldset>
            <legend>Qual é a história?</legend>
            <div className="share-hand-kinds">
              <label className={kind === 'brag' ? 'is-selected' : undefined}>
                <input type="radio" name="share-kind" checked={kind === 'brag'}
                       onChange={() => setKind('brag')}/>
                <span className="share-hand-kind-icon" aria-hidden="true"><Trophy/></span>
                <span><b>Brag</b><small>Uma vitória ou leitura para celebrar.</small></span>
              </label>
              <label className={kind === 'bad_beat' ? 'is-selected' : undefined}>
                <input type="radio" name="share-kind" checked={kind === 'bad_beat'}
                       onChange={() => setKind('bad_beat')}/>
                <span className="share-hand-kind-icon" aria-hidden="true"><HeartCrack/></span>
                <span><b>Bad beat</b><small>Aquela derrota improvável que precisa ser contada.</small></span>
              </label>
            </div>
          </fieldset>
          <label className="share-hand-check">
            <input type="checkbox" checked={includeCards} onChange={event => setIncludeCards(event.target.checked)}/>
            <span><b>Mostrar minhas cartas</b><small>Desmarque para publicar somente board, resultado e ações.</small></span>
          </label>
          <label className="share-hand-expiry"><span><Clock3 aria-hidden="true"/>Link disponível por</span>
            <select value={expiryDays} onChange={event => setExpiryDays(Number(event.target.value))}>
              <option value={1}>24 horas</option>
              <option value={7}>7 dias</option>
              <option value={30}>30 dias</option>
            </select>
          </label>
          <p className="share-hand-privacy"><ShieldCheck aria-hidden="true"/>
            <span><b>Você controla o que sai da mesa.</b> O link expira sozinho e nunca revela cartas não mostradas.</span>
          </p>
        </div> : <div className="share-hand-created" role="status" aria-live="polite">
          <span className="share-hand-success-mark" aria-hidden="true"><Check/></span>
          <b>{justCreated ? 'Link criado. Mão pronta para circular.' : 'Você já tem um link para esta mão.'}</b>
          <p>Quem tiver o endereço poderá abrir esta versão anonimizada até ela expirar.</p>
          <div className="share-hand-link"><input readOnly value={url} aria-label="Link compartilhável"
                                                   onFocus={event => event.currentTarget.select()}/>
            <Button type="button" onClick={copy}>{copied ? <Check/> : <Copy/>}{copied ? 'Copiado' : 'Copiar'}</Button>
          </div>
        </div>}
        {error && <p className="share-hand-error" role="alert"><CircleAlert aria-hidden="true"/>{error}</p>}
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Fechar</Button>
          {url && <Button type="button" variant="destructive" disabled={revoking} onClick={revoke}>
            {revoking ? <LoaderCircle className="spin"/> : <ShieldOff/>} {revoking ? 'Revogando…' : 'Revogar'}
          </Button>}
          {!url && <Button type="button" disabled={pending} onClick={create}>
            {pending ? <LoaderCircle className="spin"/> : <Share2/>} {pending ? 'Criando…' : 'Criar link'}
          </Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
