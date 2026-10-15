import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {ResponsibleStatus} from '@/lib/api/responsible';
import {ResponsibleSection, toCentavos} from './ResponsibleSection';

const api = vi.hoisted(() => ({
  getResponsible: vi.fn(),
  acceptGamblingAddendum: vi.fn(),
  saveLimits: vi.fn(),
  cancelPendingLimits: vi.fn(),
  selfExclude: vi.fn(),
  revokeSelfExclusion: vi.fn(),
}));
vi.mock('@/lib/api/responsible', async importOriginal => ({...await importOriginal<typeof import('@/lib/api/responsible')>(), ...api}));
const {notify} = vi.hoisted(() => ({notify: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));

function wrapper({children}: {children: ReactNode}) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const usage = {daily: 0, weekly: 0, monthly: 0, day_resets_at: '', week_resets_at: '', month_resets_at: ''};
const base: ResponsibleStatus = {addendum_version: '2.1', accepted: true, limits: null, usage, excluded: null};
const withLimits: ResponsibleStatus = {...base, limits: {daily: 10_000, weekly: 50_000, monthly: 100_000}};

describe('ResponsibleSection', () => {
  beforeEach(() => {
    Object.values(api).forEach(fn => fn.mockReset());
    notify.mockReset();
  });

  test('toCentavos reads reais, with a comma or a dot, and refuses anything that is not a positive amount', () => {
    expect(toCentavos('100')).toBe(10_000);
    expect(toCentavos('12,34')).toBe(1_234);
    expect(toCentavos('12.34')).toBe(1_234);
    expect([toCentavos(''), toCentavos('-5'), toCentavos('abc')]).toEqual([0, 0, 0]);
  });

  test('loads, then offers only the consent until it is given', async () => {
    api.getResponsible.mockResolvedValue({...base, accepted: false});
    api.acceptGamblingAddendum.mockResolvedValue(undefined);
    render(<ResponsibleSection/>, {wrapper});

    expect(screen.getByText('Carregando seu jogo responsável…')).toBeInTheDocument();
    const accept = await screen.findByRole('button', {name: 'Aceitar o termo'});
    expect(screen.queryByRole('button', {name: 'Salvar limites'})).not.toBeInTheDocument();
    await userEvent.click(accept);
    expect(api.acceptGamblingAddendum).toHaveBeenCalledOnce();
  });

  test('shows an error with a retry when the state cannot be loaded', async () => {
    api.getResponsible.mockRejectedValue(new Error('network'));
    render(<ResponsibleSection/>, {wrapper});
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar o jogo responsável agora.');
    expect(screen.getByRole('button', {name: 'Tentar novamente'})).toBeInTheDocument();
  });

  test('the first limits are asked for, must be in order, and are sent in centavos', async () => {
    api.getResponsible.mockResolvedValue(base);
    api.saveLimits.mockResolvedValue({daily: 5_000, weekly: 20_000, monthly: 80_000});
    render(<ResponsibleSection/>, {wrapper});

    expect(await screen.findByText('Defina seus três limites antes de jogar com dinheiro real.')).toBeInTheDocument();
    const save = screen.getByRole('button', {name: 'Salvar limites'});
    await userEvent.type(screen.getByLabelText('Limite diário (R$)'), '50');
    await userEvent.type(screen.getByLabelText('Limite semanal (R$)'), '20');
    await userEvent.type(screen.getByLabelText('Limite mensal (R$)'), '800');
    expect(screen.getByRole('alert')).toHaveTextContent('O limite diário não pode passar do semanal');
    expect(save).toBeDisabled();

    await userEvent.clear(screen.getByLabelText('Limite semanal (R$)'));
    await userEvent.type(screen.getByLabelText('Limite semanal (R$)'), '200');
    await userEvent.click(save);
    expect(api.saveLimits).toHaveBeenCalledWith({daily: 5_000, weekly: 20_000, monthly: 80_000}, expect.anything());
  });

  test('says an increase waits and a decrease does not, and shows a scheduled increase with a way to cancel it', async () => {
    api.getResponsible.mockResolvedValue({
      ...withLimits,
      limits: {...withLimits.limits!, pending: {daily: 20_000, weekly: 50_000, monthly: 100_000, applies_at: '2026-10-22T12:00:00Z'}},
    });
    api.cancelPendingLimits.mockResolvedValue(withLimits.limits);
    render(<ResponsibleSection/>, {wrapper});

    const daily = await screen.findByLabelText('Limite diário (R$)');
    expect(daily).toHaveValue(100);
    expect(screen.queryByText(/só vale depois de 7 dias/)).not.toBeInTheDocument();
    await userEvent.clear(daily);
    await userEvent.type(daily, '150');
    expect(screen.getByText(/só vale depois de 7 dias/)).toBeInTheDocument();

    expect(screen.getByText(/Aumento agendado para/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar aumento'}));
    expect(api.cancelPendingLimits).toHaveBeenCalledOnce();
  });

  test('a self-exclusion asks for confirmation before it is made', async () => {
    api.getResponsible.mockResolvedValue(withLimits);
    api.selfExclude.mockResolvedValue({period: '90d', requested_at: '', until: '2027-01-13T00:00:00Z'});
    render(<ResponsibleSection/>, {wrapper});

    await userEvent.selectOptions(await screen.findByLabelText('Pausar por'), '90d');
    await userEvent.click(screen.getByRole('button', {name: 'Pausar'}));
    expect(api.selfExclude).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('Você não poderá encurtá-la');
    await userEvent.click(screen.getByRole('button', {name: 'Confirmar pausa'}));
    expect(api.selfExclude).toHaveBeenCalledWith('90d', expect.anything());
  });

  test('an active indefinite exclusion can be ended; a timed one cannot', async () => {
    api.getResponsible.mockResolvedValue({...withLimits, excluded: {period: 'indefinite', requested_at: '2026-01-01T00:00:00Z'}});
    api.revokeSelfExclusion.mockResolvedValue(undefined);
    const first = render(<ResponsibleSection/>, {wrapper});
    await userEvent.click(await screen.findByRole('button', {name: 'Encerrar a pausa'}));
    expect(api.revokeSelfExclusion).toHaveBeenCalledOnce();
    first.unmount();

    api.getResponsible.mockResolvedValue({...withLimits, excluded: {period: '30d', requested_at: '', until: '2027-01-01T00:00:00Z'}});
    render(<ResponsibleSection/>, {wrapper});
    expect(await screen.findByText(/Você está em pausa até/)).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Encerrar a pausa'})).not.toBeInTheDocument();
  });
});
