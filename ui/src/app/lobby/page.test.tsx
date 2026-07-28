import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  remainingTime: vi.fn(),
  spin: vi.fn(),
  notify: vi.fn(),
  invalidateQueries: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({invalidateQueries: mocks.invalidateQueries}),
}));
vi.mock('@/lib/api/gamification', () => ({
  remainingTime: mocks.remainingTime,
  spin: mocks.spin,
}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: true}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));
vi.mock('@/components/lobby/StakesGrid', () => ({StakesGrid: () => <div>stakes-grid</div>}));
vi.mock('@/components/lobby/ActiveTableBanner', () => ({ActiveTableBanner: () => <div>active-table</div>}));
vi.mock('@/components/lobby/CreateRoomDialog', () => ({CreateRoomDialog: () => <div>create-room</div>}));
vi.mock('@/components/lobby/OnboardingIntro', () => ({OnboardingIntro: () => <div>onboarding</div>}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/components/table/MockControls', () => ({MockControls: () => <div>mock-controls</div>}));

import Lobby from './page';

describe('lobby page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.remainingTime.mockResolvedValue({remaining_time_seconds: 0});
  });

  test('composes the primary lobby entry points and enables a ready reward', async () => {
    render(<Lobby/>);
    expect(screen.getByRole('heading', {name: 'Escolha sua mesa.'})).toBeInTheDocument();
    expect(screen.getByText('stakes-grid')).toBeInTheDocument();
    expect(screen.getByText('active-table')).toBeInTheDocument();
    expect(await screen.findByText('mock-controls')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', {name: /Recompensa Diária/})).toBeEnabled());
  });

  test('shows and decrements the daily reward cooldown', async () => {
    vi.useFakeTimers();
    mocks.remainingTime.mockResolvedValue({remaining_time_seconds: 61});
    render(<Lobby/>);
    await act(async () => {});
    expect(screen.getByText('01:01')).toBeInTheDocument();
    await act(async () => vi.advanceTimersByTime(1000));
    expect(screen.getByText('01:00')).toBeInTheDocument();
    vi.useRealTimers();
  });

  test('claims chips, refreshes the profile and reports the result', async () => {
    mocks.spin.mockResolvedValue({amount: 1500, remaining_time_seconds: 86400});
    render(<Lobby/>);
    const reward = await screen.findByRole('button', {name: /Recompensa Diária/});
    await waitFor(() => expect(reward).toBeEnabled());
    fireEvent.click(reward);
    expect(await screen.findByText('24h 00min')).toBeInTheDocument();
    expect(mocks.notify).toHaveBeenCalledWith('Você ganhou +1.500 fichas sandbox!', 'info');
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['player', 'me']});
  });

  test('reports cooldown-only and failed claims', async () => {
    mocks.spin.mockResolvedValueOnce({amount: 0, remaining_time_seconds: 3661});
    const first = render(<Lobby/>);
    fireEvent.click(await screen.findByRole('button', {name: /Recompensa Diária/}));
    await waitFor(() => expect(mocks.notify)
      .toHaveBeenCalledWith('Recompensa disponível em 1h 01min.', 'info'));
    first.unmount();

    mocks.remainingTime.mockResolvedValueOnce({remaining_time_seconds: 0});
    mocks.spin.mockRejectedValueOnce(new Error('offline'));
    render(<Lobby/>);
    fireEvent.click(await screen.findByRole('button', {name: /Recompensa Diária/}));
    await waitFor(() => expect(mocks.notify)
      .toHaveBeenCalledWith('Não foi possível resgatar a recompensa agora.', 'error'));
  });

  test('falls back to a claimable reward when loading the cooldown fails', async () => {
    mocks.remainingTime.mockRejectedValue(new Error('offline'));
    render(<Lobby/>);
    await waitFor(() => expect(screen.getByRole('button', {name: /Recompensa Diária/})).toBeEnabled());
  });
});
