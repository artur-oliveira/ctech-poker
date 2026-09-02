import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileMenu} from './ProfileMenu';
import type {CosmeticCatalogEntry, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutate: vi.fn(),
  setQueryData: vi.fn(),
  invalidateQueries: vi.fn(),
  logout: vi.fn(),
  notify: vi.fn(),
  saveShouldFail: false,
  realMoney: {enabled: false},
  state: {
    player: undefined as unknown,
    catalog: [] as CosmeticCatalogEntry[],
    purchases: [] as CosmeticPurchase[],
  },
}));

vi.mock('@/lib/capabilities', () => ({
  get REAL_MONEY_UI_ENABLED() {
    return mocks.realMoney.enabled;
  },
  availableWalletMode: (value: string | null | undefined) =>
    value === 'real' && mocks.realMoney.enabled ? 'real' : 'sandbox',
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({queryKey}: {queryKey: unknown[]}) => {
    mocks.query(queryKey);
    if (queryKey[0] === 'player') return {data: mocks.state.player};
    if (queryKey[1] === 'cosmetic-catalog') return {data: mocks.state.catalog};
    if (queryKey[1] === 'cosmetic-purchases') return {data: mocks.state.purchases};
    return {data: undefined};
  },
  useQueryClient: () => ({setQueryData: mocks.setQueryData, invalidateQueries: mocks.invalidateQueries}),
  useMutation: ({onSuccess, onError}: {
    onSuccess: (data: unknown, input: unknown) => void;
    onError?: (error: unknown, input: unknown) => void;
  }) => ({
    mutate: (input: unknown) => {
      mocks.mutate(input);
      if (mocks.saveShouldFail) {
        onError?.(new Error('rejected'), input);
        return;
      }
      onSuccess({...(mocks.state.player as object), ...(input as object)}, input);
    },
    isPending: false,
  }),
}));
vi.mock('@/lib/auth/oauth', () => ({logout: mocks.logout}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/lib/avatar', () => ({uploadAvatar: vi.fn(), deleteAvatar: vi.fn()}));
vi.mock('next/image', () => ({
  default: (props: { src: string; alt: string }) => <span role="img" aria-label={props.alt} data-src={props.src}/>,
}));
vi.mock('@/components/lobby/ProfileShowcaseDialog', () => ({
  ProfileShowcaseDialog: ({open}: { open: boolean }) => open ? <div>showcase-open</div> : null,
}));
vi.mock('@/components/lobby/SelfHudDialog', () => ({
  SelfHudDialog: ({open}: { open: boolean }) => open ? <div>hud-open</div> : null,
}));

const player = {
  user_id: 'player-1',
  name: 'Ana Silva',
  wallet_mode: 'sandbox',
  poker_terms_accepted: true,
  sandbox_balance: 12_345,
  game_balance: 987.6,
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
    mocks.state.catalog = [];
    mocks.state.purchases = [];
    mocks.saveShouldFail = false;
    mocks.realMoney.enabled = false;
  });

  test('summarizes the sandbox wallet in the menu', async () => {
    render(<ProfileMenu/>);
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: /Abrir loja/})).toHaveAttribute('href', '/store');
    expect(screen.getByText('AS')).toBeInTheDocument();

    await openProfile();
    expect(screen.getAllByText('12.345 fichas')).toHaveLength(2);
    expect(screen.getByRole('button', {name: /Loja/})).toHaveAttribute('href', '/store');
  });

  test('hides the wallet-mode switch and the real-money balance when real money is off', async () => {
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<ProfileMenu/>);
    // A server-set real wallet mode must not leak into the pill.
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();

    await openProfile();
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    expect(screen.queryByText('Modo de jogo')).not.toBeInTheDocument();
    expect(screen.queryByText('Dinheiro real')).not.toBeInTheDocument();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
  });

  test('shows both balances and the wallet-mode switch when real money is on', async () => {
    mocks.realMoney.enabled = true;
    render(<ProfileMenu/>);
    await openProfile();
    expect(screen.getByRole('switch', {name: 'Sandbox'})).toBeInTheDocument();
    expect(screen.getByText('Modo de jogo')).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
  });

  test('reverts and explains when a wallet-mode change is rejected', async () => {
    mocks.realMoney.enabled = true;
    mocks.saveShouldFail = true;
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('switch', {name: 'Sandbox'}));
    expect(mocks.mutate).toHaveBeenCalledWith({wallet_mode: 'real'});
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['player', 'me']});
    expect(mocks.notify).toHaveBeenCalledWith(
      'Não foi possível trocar o modo de jogo. Seu modo atual foi mantido.'
    );
  });

  test('trims and saves a changed display name into the player cache', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Ana Silva'}));
    const input = screen.getByRole('textbox', {name: 'Nome de exibição'});
    await userEvent.clear(input);
    await userEvent.type(input, '  Nova Ana  ');
    await userEvent.click(screen.getByRole('button', {name: 'Salvar'}));

    expect(mocks.mutate).toHaveBeenCalledWith({name: 'Nova Ana'});
    expect(mocks.setQueryData).toHaveBeenCalledWith(
      ['player', 'me'],
      expect.objectContaining({name: 'Nova Ana'})
    );
    await waitFor(() => expect(screen.queryByRole('textbox')).not.toBeInTheDocument());
  });

  test('cancels name editing with Escape and prevents an empty save', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Ana Silva'}));
    const input = screen.getByRole('textbox', {name: 'Nome de exibição'});
    await userEvent.clear(input);
    expect(screen.getByRole('button', {name: 'Salvar'})).toBeDisabled();
    await userEvent.type(input, '{Escape}');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(mocks.mutate).not.toHaveBeenCalled();
  });

  test('switches to real money, opens profile tools, and logs out', async () => {
    mocks.realMoney.enabled = true;
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('switch', {name: 'Sandbox'}));
    expect(mocks.mutate).toHaveBeenCalledWith({wallet_mode: 'real'});

    await userEvent.click(screen.getByRole('button', {name: 'Vitrine do perfil'}));
    expect(screen.getByText('showcase-open')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Seu jogo/}));
    expect(screen.getByText('hud-open')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Sair da conta/}));
    expect(mocks.logout).toHaveBeenCalledOnce();
  });


  test('saves the display name straight from the Enter key', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Ana Silva'}));
    await userEvent.type(screen.getByRole('textbox', {name: 'Nome de exibição'}), '{Enter}');
    expect(mocks.mutate).toHaveBeenCalledWith({name: 'Ana Silva'});
    expect(mocks.notify).toHaveBeenCalledWith('Agora você joga como Ana Silva.', 'info');
  });

  test('confirms a deck change and a switch back to sandbox', async () => {
    mocks.realMoney.enabled = true;
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('switch', {name: 'Dinheiro real'}));
    expect(mocks.mutate).toHaveBeenCalledWith({wallet_mode: 'sandbox'});
    expect(mocks.notify).toHaveBeenCalledWith('Modo sandbox selecionado.', 'info');

    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));
    await userEvent.click(await screen.findByRole('option', {name: /Clássico/}));
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'two-color'});
    expect(mocks.notify).toHaveBeenCalledWith('Baralho pronto para a próxima mão.', 'info');
  });

  test('uploads a chosen profile photo and lets an existing one be removed', async () => {
    mocks.state.player = {...player, avatar_url: '/avatars/player-1.jpg'};
    render(<ProfileMenu/>);
    await openProfile();

    const file = new File(['photo'], 'photo.png', {type: 'image/png'});
    await userEvent.upload(screen.getByLabelText('Selecionar foto de perfil'), file);
    expect(mocks.mutate).toHaveBeenCalledWith(file);

    expect(screen.getByRole('button', {name: 'Trocar foto de perfil'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Remover foto de perfil'}));
    expect(mocks.notify).toHaveBeenCalledWith('Foto de perfil removida.', 'info');
  });

  test('offers to add a photo, and no removal, when the player has none', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    expect(screen.getByRole('button', {name: 'Adicionar foto de perfil'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Remover foto de perfil'})).not.toBeInTheDocument();
  });

  test('falls back to zeroed balances and an unset name for a fresh player', async () => {
    mocks.realMoney.enabled = true;
    mocks.state.player = undefined;
    render(<ProfileMenu/>);
    expect(screen.getByText('0 fichas')).toBeInTheDocument();

    await openProfile();
    expect(screen.getByRole('button', {name: /Definir nome/})).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*0,00/)).toBeInTheDocument();
    expect(screen.getByText('Vitrine privada')).toBeInTheDocument();
  });

  test('formats the real-money wallet in the collapsed summary when real money is on', () => {
    mocks.realMoney.enabled = true;
    mocks.state.player = {...player, wallet_mode: 'real'};
    render(<ProfileMenu/>);
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
  });

  test('locks an unowned premium deck with a link to the store instead of selecting it', async () => {
    mocks.state.catalog = [{kind: 'deck', id: 'golden', premium: true, owned: false, price_fichas: 500_000}];
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));

    const golden = await screen.findByRole('option', {name: /Dourado/});
    expect(golden.tagName).toBe('A');
    expect(golden).toHaveAttribute('href', '/store#decks');
    expect(golden.querySelector('svg[aria-label*="Baralho premium bloqueado"]')).not.toBeNull();
    expect(golden.querySelector('svg[aria-label*="500.000 fichas"]')).not.toBeNull();
    expect(mocks.mutate).not.toHaveBeenCalledWith(expect.objectContaining({deck_variant: 'golden'}));
  });

  test('an owned premium deck selects normally, with no lock icon', async () => {
    // Ownership is a catalog fact (the server reads it from entitlements), not a
    // purchase-history one.
    mocks.state.catalog = [{kind: 'deck', id: 'golden', premium: true, owned: true, price_fichas: 500_000}];
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('combobox', {name: 'Baralho'}));

    const golden = await screen.findByRole('option', {name: 'Dourado'});
    expect(golden.tagName).not.toBe('A');
    await userEvent.click(golden);
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'golden'});
  });
});
