'use client';
import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Button, Field, Input} from '@aoctech/ui';
import {
  acceptGamblingAddendum,
  cancelPendingLimits,
  GAMBLING_ADDENDUM_URL,
  getResponsible,
  revokeSelfExclusion,
  RESPONSIBLE_KEY,
  saveLimits,
  selfExclude,
  type ExclusionPeriod,
  type ResponsibleStatus
} from '@/lib/api/responsible';
import {moneyExact} from '@/lib/chips';
import {pushNotification} from '@/lib/notify';

const PERIOD_LABELS: Record<ExclusionPeriod, string> = {
  '30d': '30 dias',
  '90d': '90 dias',
  indefinite: 'Tempo indeterminado'
};

const dateFormat = new Intl.DateTimeFormat('pt-BR', {dateStyle: 'long'});
const formatDate = (iso: string) => dateFormat.format(new Date(iso));

/** Reais typed by the player to integer centavos; anything not a positive amount is 0. */
export function toCentavos(reais: string): number {
  const value = Number(reais.replace(',', '.'));
  return Number.isFinite(value) && value > 0 ? Math.round(value * 100) : 0;
}

/**
 * The player's consent, personal limits and self-exclusion for real-money play. They used to
 * live in the wallet; the rules are Poker's now (api/internal/responsible). Shown only where
 * real money is on, and it only ever reduces exposure: nothing here lets a limit be raised
 * without its cooling-off, or an exclusion be cut short.
 */
export function ResponsibleSection() {
  const query = useQuery({queryKey: RESPONSIBLE_KEY, queryFn: getResponsible});

  return <section className="player-profile-section" aria-labelledby="player-profile-responsible-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-responsible-title">Jogo responsável</h2>
      <p>Seus limites e sua pausa para jogar com dinheiro real. Diminuir um limite vale na hora; aumentar espera alguns dias.</p>
    </header>
    {query.isLoading
      ? <p className="field-description">Carregando seu jogo responsável…</p>
      : query.isError || !query.data
        ? <p className="form-error" role="alert">Não foi possível carregar o jogo responsável agora.
          <Button type="button" variant="ghost" size="sm" onClick={() => void query.refetch()}>Tentar novamente</Button></p>
        : <ResponsibleBody status={query.data}/>}
  </section>;
}

function useResponsibleMutation<T, V = void>(fn: (v: V) => Promise<T>, done: string, failed: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({queryKey: RESPONSIBLE_KEY});
      pushNotification(done, 'info');
    },
    onError: () => pushNotification(failed)
  });
}

function ResponsibleBody({status}: {status: ResponsibleStatus}) {
  const accept = useResponsibleMutation(acceptGamblingAddendum, 'Termo aceito.', 'Não foi possível registrar o aceite agora. Tente novamente.');

  return <>
    <div className="player-profile-responsible-block">
      {status.accepted
        ? <p role="status">Você aceitou o termo de jogo responsável (versão {status.addendum_version}).</p>
        : <>
          <p>Para jogar com dinheiro real você precisa aceitar o <a href={GAMBLING_ADDENDUM_URL} target="_blank" rel="noreferrer">termo de jogo responsável</a>.</p>
          <Button type="button" onClick={() => accept.mutate()} disabled={accept.isPending} loading={accept.isPending}>
            Aceitar o termo
          </Button>
        </>}
    </div>
    {status.accepted && <>
      <LimitsForm key={limitsKey(status)} status={status}/>
      <ExclusionBlock status={status}/>
    </>}
  </>;
}

const limitsKey = (s: ResponsibleStatus) =>
  `${s.limits?.daily}:${s.limits?.weekly}:${s.limits?.monthly}:${s.limits?.pending?.applies_at ?? ''}`;

function LimitsForm({status}: {status: ResponsibleStatus}) {
  const limits = status.limits;
  const reais = (c?: number) => (c ? String(c / 100) : '');
  const [daily, setDaily] = useState(reais(limits?.daily));
  const [weekly, setWeekly] = useState(reais(limits?.weekly));
  const [monthly, setMonthly] = useState(reais(limits?.monthly));
  const save = useResponsibleMutation(saveLimits, 'Limites salvos.', 'Não foi possível salvar os limites agora. Confira os valores e tente novamente.');
  const cancel = useResponsibleMutation(cancelPendingLimits, 'Aumento cancelado.', 'Não foi possível cancelar o aumento agora.');

  const [d, w, m] = [toCentavos(daily), toCentavos(weekly), toCentavos(monthly)];
  const ordered = d > 0 && w >= d && m >= w;
  const isIncrease = Boolean(limits) && (d > (limits?.daily ?? 0) || w > (limits?.weekly ?? 0) || m > (limits?.monthly ?? 0));

  return <form className="player-profile-responsible-block" onSubmit={event => {
    event.preventDefault();
    if (ordered) save.mutate({daily: d, weekly: w, monthly: m});
  }}>
    <h3>Limites de entrada</h3>
    {!limits && <p role="status">Defina seus três limites antes de jogar com dinheiro real.</p>}
    <p className="field-description">O que você pode levar para as mesas de dinheiro real em cada período. Sacar da mesa não devolve limite.</p>
    <Field label="Limite diário (R$)">
      {control => <Input {...control} type="number" inputMode="decimal" min={0} step="0.01" value={daily}
                         onChange={event => setDaily(event.target.value)}/>}
    </Field>
    <Field label="Limite semanal (R$)">
      {control => <Input {...control} type="number" inputMode="decimal" min={0} step="0.01" value={weekly}
                         onChange={event => setWeekly(event.target.value)}/>}
    </Field>
    <Field label="Limite mensal (R$)">
      {control => <Input {...control} type="number" inputMode="decimal" min={0} step="0.01" value={monthly}
                         onChange={event => setMonthly(event.target.value)}/>}
    </Field>
    {!ordered && (daily || weekly || monthly) && <p className="form-error" role="alert">O limite diário não pode passar do semanal, nem o semanal do mensal.</p>}
    {isIncrease && ordered && <p role="status">Aumentar um limite só vale depois de 7 dias (14 se for o mensal). Diminuir vale na hora.</p>}
    {limits && <p className="field-description">
      Usado hoje {moneyExact(status.usage.daily)}, na semana {moneyExact(status.usage.weekly)}, no mês {moneyExact(status.usage.monthly)}.
    </p>}
    {limits?.pending && <p role="status">
      Aumento agendado para {formatDate(limits.pending.applies_at)}: {moneyExact(limits.pending.daily)} por dia.{' '}
      <Button type="button" variant="ghost" size="sm" onClick={() => cancel.mutate()} disabled={cancel.isPending}>Cancelar aumento</Button>
    </p>}
    <div className="player-profile-actions">
      <Button type="submit" disabled={!ordered || save.isPending} loading={save.isPending}>
        {save.isPending ? 'Salvando…' : 'Salvar limites'}
      </Button>
    </div>
  </form>;
}

function ExclusionBlock({status}: {status: ResponsibleStatus}) {
  const [period, setPeriod] = useState<ExclusionPeriod>('30d');
  const [confirming, setConfirming] = useState(false);
  const exclude = useResponsibleMutation(selfExclude, 'Pausa registrada.', 'Não foi possível registrar a pausa agora. Tente novamente.');
  const revoke = useResponsibleMutation(revokeSelfExclusion, 'Pausa encerrada.', 'A pausa por tempo indeterminado só pode ser encerrada após 90 dias.');
  const excluded = status.excluded;

  return <div className="player-profile-responsible-block">
    <h3>Pausa (autoexclusão)</h3>
    {excluded
      ? <>
        <p role="status">
          {excluded.until
            ? `Você está em pausa até ${formatDate(excluded.until)}. Ela só pode ser estendida, nunca encurtada.`
            : 'Você está em pausa por tempo indeterminado. Ela pode ser encerrada depois de 90 dias.'}
        </p>
        {!excluded.until && <Button type="button" variant="outline" onClick={() => revoke.mutate()} disabled={revoke.isPending}>
          Encerrar a pausa
        </Button>}
      </>
      : <p className="field-description">Uma pausa impede buy-ins em mesas de dinheiro real. As mesas de fichas continuam abertas.</p>}
    <Field label={excluded ? 'Estender a pausa para' : 'Pausar por'}>
      {control => <select {...control} value={period} onChange={event => { setPeriod(event.target.value as ExclusionPeriod); setConfirming(false); }}>
        {(Object.keys(PERIOD_LABELS) as ExclusionPeriod[]).map(p => <option key={p} value={p}>{PERIOD_LABELS[p]}</option>)}
      </select>}
    </Field>
    {confirming
      ? <p role="alert">
        Confirmar uma pausa de <b>{PERIOD_LABELS[period]}</b>? Você não poderá encurtá-la.{' '}
        <Button type="button" onClick={() => { exclude.mutate(period); setConfirming(false); }} disabled={exclude.isPending}>Confirmar pausa</Button>{' '}
        <Button type="button" variant="ghost" onClick={() => setConfirming(false)}>Voltar</Button>
      </p>
      : <Button type="button" variant="outline" onClick={() => setConfirming(true)}>
        {excluded ? 'Estender a pausa' : 'Pausar'}
      </Button>}
  </div>;
}
