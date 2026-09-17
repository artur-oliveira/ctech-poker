import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileMenu} from './ProfileMenu';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  logout: vi.fn(),
  notify: vi.fn(),
  realMoney: {enabled: false},
  state: {player: undefined as unknown},
}));

vi.mock('@/lib/capabilities', () => ({
  get REAL_MONEY_UI_ENABLED() {
    return mocks.realMoney.enabled;
  },
  availableWalletMode: (value: string | null | undefined) =>
    value === 'real' && mocks.realMoney.enabled ? 'real' : 'sandbox',
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey, enabled}: {queryKey: unknown[]; enabled?: boolean}) => {
    if (enabled === false) return {data: undefined};
    mocks.query(queryKey);
    if (queryKey[0] === 'player') return {data: mocks.state.player};
    return {data: undefined};
  },
}));
vi.mock('@/lib/auth/oauth', () => ({logout: mocks.logout, endSession: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/components/lobby/SelfHudDialog', () => ({
  SelfHudDialog: ({open}: {open: boolean}) => open ? <div>hud-open</div> : null,
}));

const player = {
  user_id: 'player-1',
  name: 'Ana Silva',
  wallet_mode: 'sandbox',
  poker_terms_accepted: true,
  sandbox_balance: 12_345,
  game_balance: 98_760,
  showcase_public: true,
};

async function openProfile() {
  await userEvent.click(screen.getByRole('button', {name: 'Abrir perfil'}));
  await screen.findByText('Nome de exibição');
}

describe('ProfileMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.player = player;
    mocks.realMoney.enabled = false;
  });

  test('summarizes the wallet in the menu', async () => {
    render(<ProfileMenu/>);
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: /Abrir loja/})).toHaveAttribute('href', '/store');
    expect(screen.getByText('AS')).toBeInTheDocument();

    await openProfile();
    // The pill carries the unit; the panel's own row is labelled "Fichas".
    expect(screen.getByText('12.345')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Loja/})).toHaveAttribute('href', '/store');
  });

  // Request budget: the popover is mounted on every authenticated page, and it
  // now reads nothing the shell did not already have. The deck catalog moved
  // to /player-profile with the picker that needed it.
  test('reads only the shared profile, never a catalog', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    expect(mocks.query).toHaveBeenCalledWith(['player', 'me']);
    expect(mocks.query).not.toHaveBeenCalledWith(['wallet', 'cosmetic-catalog', 'deck']);
    expect(mocks.query.mock.calls).toHaveLength(1);
  });

  // Every dense editor moved to the route; two sources of truth for the same
  // field is the bug this replaced.
  test('edits nothing itself and points at the profile route instead', async () => {
    render(<ProfileMenu/>);
    await openProfile();

    expect(screen.getByText('Ana Silva')).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', {name: 'Baralho'})).not.toBeInTheDocument();
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Selecionar foto de perfil')).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Editar perfil/})).toHaveAttribute('href', '/player-profile');
  });

  test('hides the real-money balance when real money is off', async () => {
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<ProfileMenu/>);
    // A server-set real wallet mode must not leak into the pill.
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();

    await openProfile();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
    expect(screen.queryByText('Dinheiro real')).not.toBeInTheDocument();
    expect(screen.getByText('12.345')).toBeInTheDocument();
  });

  test('shows both balances when real money is on', async () => {
    mocks.realMoney.enabled = true;
    render(<ProfileMenu/>);
    await openProfile();
    expect(screen.getByText('Dinheiro real')).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
  });

  test('formats the real-money wallet in the collapsed summary when real money is on', () => {
    mocks.realMoney.enabled = true;
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<ProfileMenu/>);
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
  });

  test('opens the self HUD and logs out', async () => {
    render(<ProfileMenu/>);
    await openProfile();

    await userEvent.click(screen.getByRole('button', {name: /Seu jogo/}));
    expect(screen.getByText('hud-open')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Sair da conta/}));
    expect(mocks.logout).toHaveBeenCalledOnce();
    // A second press cannot fire a second revoke.
    await userEvent.click(screen.getByRole('button', {name: /Saindo…/}));
    expect(mocks.logout).toHaveBeenCalledOnce();
  });

  test('falls back to zeroed balances and an unset name for a fresh player', async () => {
    mocks.realMoney.enabled = true;
    mocks.state.player = undefined;
    render(<ProfileMenu/>);
    expect(screen.getByText('0 fichas')).toBeInTheDocument();

    await openProfile();
    expect(screen.getByText('0')).toBeInTheDocument();
    expect(screen.getByText('Sem nome ainda')).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*0,00/)).toBeInTheDocument();
    expect(screen.getByText('Vitrine privada')).toBeInTheDocument();
  });
});
