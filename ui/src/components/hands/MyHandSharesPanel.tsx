'use client';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Check, ChevronDown, Clock3, Copy, HeartCrack, LoaderCircle, Link2, ShieldOff, Trophy} from 'lucide-react';
import {useState} from 'react';
import {Button} from '@/components/ui/button';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {SkeletonList} from '@/components/ui/skeleton';
import {
  HAND_SHARES_QUERY_KEY, type HandShareSummary, listMyHandShares, revokeHandShare
} from '@/lib/api/handShares';
import {clearPersistedHandShareByToken} from '@/lib/handShareStorage';
import {relativeTime} from '@/lib/utils';

function shareURL(token: string) {
  return `${window.location.origin}/share?id=${encodeURIComponent(token)}`;
}

function ShareRow({share, onRevokeAction, revoking}: {
  share: HandShareSummary;
  revoking: boolean;
  onRevokeAction: (token: string) => void;
}) {
  const [copied, setCopied] = useState(false);
  const brag = share.kind !== 'bad_beat';

  async function copy() {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(shareURL(share.token));
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return <li className="hand-share-row">
    <span className={`hand-share-kind${brag ? '' : ' is-bad-beat'}`}>
      {brag ? <Trophy aria-hidden="true"/> : <HeartCrack aria-hidden="true"/>}
      {brag ? 'Brag' : 'Bad beat'}
    </span>
    <span className="hand-share-outcome">
      <OutcomeBadge outcome={share.outcome}/>
      <b className={share.net_change > 0 ? 'gain' : share.net_change < 0 ? 'loss' : 'even'}>
        {share.net_change > 0 ? '+' : ''}{share.net_change.toLocaleString('pt-BR')}
      </b>
    </span>
    <span className="hand-share-dates">
      <small>Criado {relativeTime(share.created_at)}</small>
      <small><Clock3 aria-hidden="true"/>Expira {relativeTime(share.expires_at)}</small>
    </span>
    <span className="hand-share-actions">
      <Button type="button" variant="ghost" size="sm" onClick={() => void copy()}>
        {copied ? <Check aria-hidden="true"/> : <Copy aria-hidden="true"/>}
        {copied ? 'Copiado' : 'Copiar link'}
      </Button>
      <Button type="button" variant="destructive" size="sm" disabled={revoking}
              onClick={() => onRevokeAction(share.token)}>
        {revoking ? <LoaderCircle className="spin" aria-hidden="true"/> : <ShieldOff aria-hidden="true"/>}
        {revoking ? 'Revogando…' : 'Revogar'}
      </Button>
    </span>
  </li>;
}

/**
 * "Meus links compartilhados" (#96): every live share the player created, on
 * any device, with a Revogar per row. `revokeHandShare` existed but was only
 * reachable from `ShareHandDialog` reopened on the same hand in the same
 * browser, which left a regretted link circulating for its whole 1–30 day TTL.
 *
 * It sits above the infinite hand list, collapsed to one line with its live
 * count: at the bottom of the page it was only reachable once the list had
 * auto-loaded the player's entire history. The heading stays in the tree in
 * every state; the toggle inside it is the WAI-ARIA accordion pattern.
 */
export function MyHandSharesPanel() {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const shares = useQuery({queryKey: HAND_SHARES_QUERY_KEY, queryFn: listMyHandShares});
  const revoke = useMutation({
    mutationFn: async (token: string) => {
      await revokeHandShare(token);
      return token;
    },
    onSuccess: token => {
      // The dialog's local memory of "I already shared this hand" is keyed by
      // hand, not token, so it has to be dropped here too — otherwise
      // reopening that hand offers a link the server has already forgotten.
      clearPersistedHandShareByToken(token);
      return queryClient.invalidateQueries({queryKey: HAND_SHARES_QUERY_KEY});
    }
  });

  const count = shares.data?.length ?? 0;
  const status = shares.isLoading ? null
    : shares.isError ? 'Não carregou'
      : count === 0 ? 'Nenhum ativo'
        : `${count} ${count === 1 ? 'ativo' : 'ativos'}`;

  return <section className={`hand-shares-panel${open ? ' is-open' : ''}`} aria-labelledby="hand-shares-heading">
    <h2 id="hand-shares-heading">
      <button type="button" className="hand-shares-toggle" aria-expanded={open} aria-controls="hand-shares-body"
              onClick={() => setOpen(current => !current)}>
        <Link2 aria-hidden="true"/>
        <span className="hand-shares-title">Meus links compartilhados</span>
        {status && <span className={`hand-shares-count${count > 0 ? ' has-links' : ''}`}>{status}</span>}
        <ChevronDown className="hand-shares-chevron" aria-hidden="true"/>
      </button>
    </h2>

    <div id="hand-shares-body" className="hand-shares-body" hidden={!open}>
      <p>Cada link abre uma versão anonimizada de uma mão até expirar. Revogar desativa o endereço na hora.</p>

      {shares.isLoading
        ? <SkeletonList label="Carregando seus links compartilhados…" count={2} height={72}
                        className="hand-shares-list"/>
        : shares.isError
          ? <div className="lobby-empty hands-state" role="alert">
            <div>
              <strong>Seus links não abriram desta vez</strong>
              <p>Os links continuam ativos. Tente carregar a lista novamente.</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => void shares.refetch()}>Tentar novamente</Button>
          </div>
          : !shares.data?.length
            ? <div className="lobby-empty hands-state">
              <div>
                <strong>Nenhum link ativo</strong>
                <p>Abra uma mão e use <b>Compartilhar</b> para criar um link com prazo. Ele aparece aqui até expirar.</p>
              </div>
            </div>
            : <ul className="hand-shares-list">
              {shares.data.map(share => <ShareRow
                key={share.token} share={share}
                revoking={revoke.isPending && revoke.variables === share.token}
                onRevokeAction={token => revoke.mutate(token)}/>)}
            </ul>}

      {revoke.isError && <p className="form-error" role="alert">
        Não foi possível revogar o link agora. Tente novamente em instantes.
      </p>}
    </div>
  </section>;
}
