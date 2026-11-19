import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {PushPreferences} from '@/lib/api/push';
import {NotificationsSection} from './NotificationsSection';

const api = vi.hoisted(() => ({
  getPushConfig: vi.fn(),
  getPushPreferences: vi.fn(),
  putPushPreferences: vi.fn(),
  sendPushTest: vi.fn(),
}));
vi.mock('@/lib/api/push', async importOriginal => ({...await importOriginal<typeof import('@/lib/api/push')>(), ...api}));
const {notify} = vi.hoisted(() => ({notify: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));

const push = vi.hoisted(() => ({
  status: 'default' as string,
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
  canInstall: false,
  install: vi.fn(),
}));
vi.mock('@/lib/providers/PushProvider', () => ({usePush: () => push}));

function wrapper({children}: {children: ReactNode}) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const enabled = {enabled: true, public_key: 'PUB', vapid_kid: 'k', max_subscriptions: 10};
const prefs: PushPreferences = {streak_reminder: {enabled: false, time: ''}, come_play: {enabled: true}};

describe('NotificationsSection', () => {
  beforeEach(() => {
    Object.values(api).forEach(fn => fn.mockReset());
    notify.mockReset();
    push.subscribe.mockReset().mockResolvedValue(undefined);
    push.unsubscribe.mockReset().mockResolvedValue(undefined);
    push.status = 'default';
    push.canInstall = false;
    api.getPushConfig.mockResolvedValue(enabled);
    api.getPushPreferences.mockResolvedValue(prefs);
  });

  test('says so, and offers nothing, when push is off on this installation', async () => {
    api.getPushConfig.mockResolvedValue({enabled: false, max_subscriptions: 10});
    render(<NotificationsSection/>, {wrapper});
    expect(await screen.findByText(/não estão disponíveis/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Ativar neste dispositivo'})).not.toBeInTheDocument();
  });

  test('asks for permission only when the button is clicked, never on load', async () => {
    render(<NotificationsSection/>, {wrapper});
    const activate = await screen.findByRole('button', {name: 'Ativar neste dispositivo'});
    expect(push.subscribe).not.toHaveBeenCalled();
    await userEvent.click(activate);
    expect(push.subscribe).toHaveBeenCalledOnce();
  });

  test('the two reminders are offered only once this device is subscribed', async () => {
    render(<NotificationsSection/>, {wrapper});
    await screen.findByRole('button', {name: 'Ativar neste dispositivo'});
    expect(screen.queryByRole('switch', {name: 'Lembrete de ofensiva'})).not.toBeInTheDocument();
  });

  test('explains a blocked permission instead of offering a button that cannot work', async () => {
    push.status = 'denied';
    render(<NotificationsSection/>, {wrapper});
    expect(await screen.findByText(/bloqueou as notificações/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Ativar neste dispositivo'})).not.toBeInTheDocument();
  });

  test('on iPhone it says to add the app to the home screen first', async () => {
    push.status = 'needs-install';
    render(<NotificationsSection/>, {wrapper});
    expect(await screen.findByText(/Adicionar à Tela de Início/i)).toBeInTheDocument();
  });

  describe('subscribed', () => {
    beforeEach(() => {
      push.status = 'subscribed';
    });

    test('shows both reminders with what the server holds', async () => {
      render(<NotificationsSection/>, {wrapper});
      expect(await screen.findByRole('switch', {name: 'Venha jogar hoje'})).toBeChecked();
      expect(screen.getByRole('switch', {name: 'Lembrete de ofensiva'})).not.toBeChecked();
    });

    test('the streak reminder needs a time: saving without one shows the error and does not call the API', async () => {
      render(<NotificationsSection/>, {wrapper});
      await userEvent.click(await screen.findByRole('switch', {name: 'Lembrete de ofensiva'}));
      await userEvent.click(screen.getByRole('button', {name: 'Salvar lembretes'}));
      expect(await screen.findByRole('alert')).toHaveTextContent('Escolha o horário do lembrete de ofensiva.');
      expect(api.putPushPreferences).not.toHaveBeenCalled();
    });

    // The time is a Select of the server's own 15-minute slots (00:00 to 23:45), not a free-typing native time input.
    test('offers the 96 slots of the day, every 15 minutes, as the only choices', async () => {
      render(<NotificationsSection/>, {wrapper});
      await userEvent.click(await screen.findByRole('switch', {name: 'Lembrete de ofensiva'}));
      await userEvent.click(screen.getByRole('combobox', {name: 'Horário do lembrete'}));
      const options = await screen.findAllByRole('option');
      expect(options).toHaveLength(96);
      expect(options[0]).toHaveTextContent('00:00');
      expect(options[1]).toHaveTextContent('00:15');
      expect(options[95]).toHaveTextContent('23:45');
    });

    test('shows the time the server holds', async () => {
      api.getPushPreferences.mockResolvedValue({streak_reminder: {enabled: true, time: '20:30'}, come_play: {enabled: true}});
      render(<NotificationsSection/>, {wrapper});
      expect(await screen.findByRole('combobox', {name: 'Horário do lembrete'})).toHaveTextContent('20:30');
    });

    test('saves the chosen time and the come-play switch', async () => {
      api.putPushPreferences.mockResolvedValue({streak_reminder: {enabled: true, time: '20:30'}, come_play: {enabled: false}});
      render(<NotificationsSection/>, {wrapper});
      await userEvent.click(await screen.findByRole('switch', {name: 'Lembrete de ofensiva'}));
      await userEvent.click(screen.getByRole('combobox', {name: 'Horário do lembrete'}));
      await userEvent.click(await screen.findByRole('option', {name: '20:30'}));
      await userEvent.click(screen.getByRole('switch', {name: 'Venha jogar hoje'}));
      await userEvent.click(screen.getByRole('button', {name: 'Salvar lembretes'}));
      expect(api.putPushPreferences).toHaveBeenCalledWith({streak_reminder: {enabled: true, time: '20:30'}, come_play: {enabled: false}});
    });

    test('sends a test push and can turn this device off', async () => {
      api.sendPushTest.mockResolvedValue(undefined);
      render(<NotificationsSection/>, {wrapper});
      await userEvent.click(await screen.findByRole('button', {name: 'Enviar teste'}));
      expect(api.sendPushTest).toHaveBeenCalledOnce();
      await userEvent.click(screen.getByRole('button', {name: 'Desativar neste dispositivo'}));
      expect(push.unsubscribe).toHaveBeenCalledOnce();
    });
  });
});
