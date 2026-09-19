'use client';
import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Field} from '@/components/ui/field';
import {Input} from '@/components/ui/input';
import {Button} from '@/components/ui/button';
import {
  deleteWalletAlertPrefs,
  getWalletAlertPrefs,
  saveWalletAlertPrefs,
  WALLET_ALERT_PREFS_KEY,
  type WalletAlertPrefs
} from '@/lib/api/walletAlerts';
import {pushNotification} from '@/lib/notify';

/**
 * #304: the player's own wallet-alert thresholds. Notification-only — never a
 * purchase or buy-in gate (see api/CLAUDE.md's walletalert package) — so this
 * section only ever configures what GET /players/me may later report back on
 * `wallet_alerts`, rendered by BalancesSection above it.
 */
export function WalletAlertsSection() {
  const prefsQuery = useQuery({queryKey: WALLET_ALERT_PREFS_KEY, queryFn: getWalletAlertPrefs});

  return <section className="player-profile-section" aria-labelledby="player-profile-wallet-alerts-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-wallet-alerts-title">Alertas de carteira</h2>
      <p>Avisos no seu perfil quando o saldo ou uma compra passarem do limite. Nunca bloqueia nada.</p>
    </header>

    {prefsQuery.isLoading
      ? <p className="field-description">Carregando seus alertas…</p>
      : prefsQuery.isError || !prefsQuery.data
        ? <p className="form-error" role="alert">Não foi possível carregar seus alertas agora.
          <Button type="button" variant="ghost" size="sm" onClick={() => void prefsQuery.refetch()}>Tentar novamente</Button></p>
        // Keyed on the loaded values so a fresh save/delete response remounts
        // the form with the new server state, without a setState-in-effect —
        // the "derive during render, not an effect" rule this codebase
        // already follows (see ui/CLAUDE.md's HandOutcomeRing bullet).
        : <WalletAlertsForm key={`${prefsQuery.data.min_sandbox_balance}:${prefsQuery.data.max_purchase_cents}`}
                            prefs={prefsQuery.data}/>}
  </section>;
}

function WalletAlertsForm({prefs}: {prefs: WalletAlertPrefs}) {
  const queryClient = useQueryClient();
  const [minBalance, setMinBalance] = useState(prefs.min_sandbox_balance ? String(prefs.min_sandbox_balance) : '');
  const [maxPurchase, setMaxPurchase] = useState(prefs.max_purchase_cents ? String(prefs.max_purchase_cents / 100) : '');

  const save = useMutation({
    mutationFn: saveWalletAlertPrefs,
    onSuccess: saved => {
      queryClient.setQueryData(WALLET_ALERT_PREFS_KEY, saved);
      void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      pushNotification('Alertas de carteira salvos.', 'info');
    },
    onError: () => pushNotification('Não foi possível salvar os alertas agora. Tente novamente.')
  });

  const remove = useMutation({
    mutationFn: deleteWalletAlertPrefs,
    onSuccess: () => {
      queryClient.setQueryData(WALLET_ALERT_PREFS_KEY, {min_sandbox_balance: 0, max_purchase_cents: 0});
      void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      pushNotification('Alertas de carteira removidos.', 'info');
    },
    onError: () => pushNotification('Não foi possível remover os alertas agora. Tente novamente.')
  });

  const hasAnyConfigured = Boolean(prefs.min_sandbox_balance || prefs.max_purchase_cents);
  const minBalanceValue = Math.max(0, Math.trunc(Number(minBalance) || 0));
  const maxPurchaseCentsValue = Math.max(0, Math.round((Number(maxPurchase) || 0) * 100));

  return <form className="player-profile-wallet-alerts" onSubmit={event => {
    event.preventDefault();
    save.mutate({min_sandbox_balance: minBalanceValue, max_purchase_cents: maxPurchaseCentsValue});
  }}>
    <Field label="Avisar quando o saldo de fichas cair abaixo de" description="Deixe em 0 para desligar este alerta.">
      {control => <Input {...control} type="number" inputMode="numeric" min={0} step={1} value={minBalance}
                         placeholder="0" onChange={event => setMinBalance(event.target.value)}/>}
    </Field>
    <Field label="Avisar quando uma compra passar de (R$)" description="Deixe em 0 para desligar este alerta.">
      {control => <Input {...control} type="number" inputMode="decimal" min={0} step="0.01" value={maxPurchase}
                         placeholder="0,00" onChange={event => setMaxPurchase(event.target.value)}/>}
    </Field>
    <div className="player-profile-actions">
      <Button type="submit" disabled={save.isPending} loading={save.isPending}>
        {save.isPending ? 'Salvando…' : 'Salvar alertas'}
      </Button>
      {hasAnyConfigured && <Button type="button" variant="ghost" disabled={remove.isPending}
                                   onClick={() => remove.mutate()}>
        {remove.isPending ? 'Removendo…' : 'Remover alertas'}
      </Button>}
    </div>
  </form>;
}
