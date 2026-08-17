'use client';
import {useId, useState} from 'react';
import {Flag} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {REPORT_CATEGORIES, type ReportCategory, type ReportSurface, reportPlayer} from '@/lib/api/social';
import {socialErrorMessage} from '@/lib/social';
import {playerName} from '@/lib/utils';

export const REPORT_CATEGORY_LABELS: Record<ReportCategory, string> = {
  harassment: 'Assédio ou ofensas',
  hate: 'Discurso de ódio',
  spam: 'Spam ou propaganda',
  cheating: 'Trapaça ou conluio',
  inappropriate_profile: 'Nome ou avatar impróprio',
  other: 'Outro motivo'
};

export const REPORT_DETAILS_MAX_LENGTH = 500;

/** The report is evidence-backed server-side: when `actionId` is present the
 * API re-reads that action from the action log and copies the sanitized text
 * itself, so nothing typed here is trusted as the chat content. */
export function ReportPlayerDialog({target, surface, tableId, handId, actionId, open, onOpenChangeAction}: {
  target: {player_id: string; name?: string} | null;
  surface: ReportSurface;
  tableId?: string;
  handId?: string;
  actionId?: string;
  open: boolean;
  onOpenChangeAction: (open: boolean) => void;
}) {
  const detailsId = useId();
  const [category, setCategory] = useState<ReportCategory>('harassment');
  const [details, setDetails] = useState('');
  const [sending, setSending] = useState(false);
  const [error, setError] = useState('');
  const [sent, setSent] = useState(false);

  async function submit() {
    if (!target) return;
    setSending(true);
    setError('');
    try {
      await reportPlayer({
        target_player_id: target.player_id, category, surface,
        table_id: tableId, hand_id: handId, action_id: actionId,
        details: details.trim() || undefined
      });
      setSent(true);
    } catch (failure) {
      setError(socialErrorMessage(failure));
    } finally {
      setSending(false);
    }
  }

  const name = target ? playerName(target.player_id, undefined, target.name) : '';
  return <Dialog open={open} onOpenChange={next => {
    onOpenChangeAction(next);
    if (!next) {
      setDetails('');
      setError('');
      setSent(false);
      setCategory('harassment');
    }
  }}>
    <DialogContent className="social-report-dialog">
      <DialogHeader>
        <DialogTitle>Denunciar {name}</DialogTitle>
        <DialogDescription>{sent
          ? 'Denúncia registrada. Nossa equipe revisa cada relato; nada é decidido automaticamente.'
          : 'A denúncia é revisada por pessoas e não avisa o jogador denunciado. Silenciar ou bloquear continua sendo a forma mais rápida de não ver esse conteúdo.'}</DialogDescription>
      </DialogHeader>
      {sent ? <DialogFooter>
        <Button type="button" onClick={() => onOpenChangeAction(false)}>Fechar</Button>
      </DialogFooter> : <>
        <fieldset className="social-report-categories">
          <legend>Motivo</legend>
          {REPORT_CATEGORIES.map(value => <label key={value}>
            <input type="radio" name="report-category" value={value} checked={category === value}
                   onChange={() => setCategory(value)}/>
            <span>{REPORT_CATEGORY_LABELS[value]}</span>
          </label>)}
        </fieldset>
        <div className="social-report-details">
          <Label htmlFor={detailsId}>Detalhes (opcional)</Label>
          <textarea id={detailsId} value={details} maxLength={REPORT_DETAILS_MAX_LENGTH} rows={3}
                    onChange={event => setDetails(event.target.value)}
                    placeholder="O que aconteceu?"/>
          <small>{details.length}/{REPORT_DETAILS_MAX_LENGTH}</small>
        </div>
        {error && <p className="social-error" role="alert">{error}</p>}
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={sending}
                  onClick={() => onOpenChangeAction(false)}>Cancelar</Button>
          <Button type="button" disabled={sending} onClick={submit}>
            <Flag aria-hidden="true"/> {sending ? 'Enviando…' : 'Enviar denúncia'}
          </Button>
        </DialogFooter>
      </>}
    </DialogContent>
  </Dialog>;
}
