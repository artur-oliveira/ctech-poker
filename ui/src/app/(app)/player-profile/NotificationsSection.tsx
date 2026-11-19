'use client';
import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Button, Field, Input, Switch} from '@aoctech/ui';
import {
  getPushConfig,
  getPushPreferences,
  putPushPreferences,
  PUSH_CONFIG_KEY,
  PUSH_PREFERENCES_KEY,
  sendPushTest,
  type PushPreferences,
} from '@/lib/api/push';
import {pushNotification} from '@/lib/notify';
import {usePush} from '@/lib/providers/PushProvider';

/** The server places reminders in 15-minute slots (go-common pushsvc), so the time input moves in 900 s steps. */
const TIME_STEP_SECONDS = 900;

/**
 * Web Push: turn notifications on for this device, then choose the two reminders (docs/specs/2026-11-18-scheduled-web-push.md).
 * Permission is asked only on the button click, never on load. The streak reminder needs a time the player picks; "venha
 * jogar hoje" is on by default and its hour is learned from when the player usually sits down.
 */
export function NotificationsSection() {
  const push = usePush();
  const queryClient = useQueryClient();
  const config = useQuery({queryKey: PUSH_CONFIG_KEY, queryFn: getPushConfig});
  const available = config.data?.enabled === true;
  const subscribed = push.status === 'subscribed';
  const prefs = useQuery({queryKey: PUSH_PREFERENCES_KEY, queryFn: getPushPreferences, enabled: available && subscribed});

  const activate = useMutation({
    mutationFn: () => push.subscribe(),
    // A new device starts with "come play" on at the server, so the preferences are read again.
    onSuccess: () => queryClient.invalidateQueries({queryKey: PUSH_PREFERENCES_KEY}),
    onError: () => pushNotification('Não foi possível ativar as notificações agora. Tente novamente.'),
  });
  const deactivate = useMutation({
    mutationFn: () => push.unsubscribe(),
    onError: () => pushNotification('Não foi possível desativar agora. Tente novamente.'),
  });
  const test = useMutation({
    mutationFn: () => sendPushTest(),
    onSuccess: () => pushNotification('Teste enviado. Ele chega em instantes.', 'info'),
    onError: () => pushNotification('Não foi possível enviar o teste agora. Tente novamente em um minuto.'),
  });
  const busy = activate.isPending || deactivate.isPending || test.isPending;

  return <section className="player-profile-section" aria-labelledby="player-profile-notifications-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-notifications-title">Notificações</h2>
      <p>Lembretes no celular ou no computador, mesmo com o jogo fechado. Você escolhe quais.</p>
    </header>

    {config.isLoading
      ? <p className="field-description">Carregando suas notificações…</p>
      : config.isError || !config.data
        ? <p className="form-error" role="alert">Não foi possível carregar as notificações agora.
          <Button type="button" variant="ghost" size="sm" onClick={() => void config.refetch()}>Tentar novamente</Button></p>
        : !available
          ? <p className="field-description">As notificações não estão disponíveis agora.</p>
          : <>
            <DeviceState status={push.status} canInstall={push.canInstall} busy={busy}
                         onActivate={() => activate.mutate()} onInstall={() => void push.install()}/>
            {subscribed && <div className="player-profile-actions">
              <Button type="button" variant="ghost" disabled={busy} onClick={() => test.mutate()}>Enviar teste</Button>
              <Button type="button" variant="ghost" disabled={busy} onClick={() => deactivate.mutate()}>
                Desativar neste dispositivo
              </Button>
            </div>}
            {subscribed && (prefs.isLoading
              ? <p className="field-description">Carregando seus lembretes…</p>
              : prefs.isError || !prefs.data
                ? <p className="form-error" role="alert">Não foi possível carregar seus lembretes agora.
                  <Button type="button" variant="ghost" size="sm" onClick={() => void prefs.refetch()}>Tentar novamente</Button></p>
                // Keyed on the loaded values so a fresh save remounts the form with the server state.
                : <RemindersForm key={`${prefs.data.streak_reminder.enabled}:${prefs.data.streak_reminder.time}:${prefs.data.come_play.enabled}`}
                                 prefs={prefs.data}/>)}
          </>}
  </section>;
}

function DeviceState({status, canInstall, busy, onActivate, onInstall}: {
  status: string; canInstall: boolean; busy: boolean; onActivate: () => void; onInstall: () => void;
}) {
  switch (status) {
    case 'loading':
      return <p className="field-description">Verificando este dispositivo…</p>;
    case 'unsupported':
      return <p className="field-description">Este navegador não permite notificações.</p>;
    case 'needs-install':
      return <p className="field-description">No iPhone, toque em Compartilhar e em Adicionar à Tela de Início; depois abra o
        CTech Poker por lá para ativar as notificações.</p>;
    case 'denied':
      return <p className="field-description">Você bloqueou as notificações para este site. Libere nas configurações do
        navegador e volte aqui.</p>;
    case 'subscribed':
      return <p className="field-description" role="status">Ativadas neste dispositivo.</p>;
    default:
      return <div className="player-profile-actions">
        {status === 'error' && <p className="form-error" role="alert">Não foi possível preparar as notificações. Tente de novo.</p>}
        <Button type="button" disabled={busy} loading={busy} onClick={onActivate}>Ativar neste dispositivo</Button>
        <small>O navegador vai pedir a sua permissão.</small>
        {canInstall && <Button type="button" variant="ghost" onClick={onInstall}>Instalar o app</Button>}
      </div>;
  }
}

function RemindersForm({prefs}: {prefs: PushPreferences}) {
  const queryClient = useQueryClient();
  const [streakOn, setStreakOn] = useState(prefs.streak_reminder.enabled);
  const [time, setTime] = useState(prefs.streak_reminder.time);
  const [playOn, setPlayOn] = useState(prefs.come_play.enabled);
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: (p: PushPreferences) => putPushPreferences(p),
    onSuccess: saved => {
      queryClient.setQueryData(PUSH_PREFERENCES_KEY, saved);
      pushNotification('Lembretes salvos.', 'info');
    },
    onError: () => pushNotification('Não foi possível salvar os lembretes agora. Tente novamente.'),
  });

  return <form className="player-profile-notifications" onSubmit={event => {
    event.preventDefault();
    if (streakOn && !time) {
      setError('Escolha o horário do lembrete de ofensiva.');
      return;
    }
    setError(null);
    save.mutate({streak_reminder: {enabled: streakOn, time}, come_play: {enabled: playOn}});
  }}>
    <fieldset className="showcase-privacy-group">
      <legend>Lembretes</legend>
      <div className="showcase-privacy-row">
        <span><b>Lembrete de ofensiva</b><small>Um aviso no horário que você escolher, se o bônus do dia ainda não foi resgatado.</small></span>
        <Switch checked={streakOn} onCheckedChange={setStreakOn} aria-label="Lembrete de ofensiva"/>
      </div>
      {streakOn && <Field label="Horário do lembrete" hint="Horário de Brasília.">
        {control => <Input {...control} type="time" step={TIME_STEP_SECONDS} compact={false} value={time}
                           onChange={event => setTime(event.target.value)}/>}
      </Field>}
      <div className="showcase-privacy-row">
        <span><b>Venha jogar hoje</b><small>Um aviso por dia, perto da hora em que você costuma jogar. Não sai se você já jogou.</small></span>
        <Switch checked={playOn} onCheckedChange={setPlayOn} aria-label="Venha jogar hoje"/>
      </div>
    </fieldset>
    {error && <p className="form-error" role="alert">{error}</p>}
    <div className="player-profile-actions">
      <Button type="submit" disabled={save.isPending} loading={save.isPending}>
        {save.isPending ? 'Salvando…' : 'Salvar lembretes'}
      </Button>
    </div>
  </form>;
}
