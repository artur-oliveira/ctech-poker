import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileMenu} from './ProfileMenu';
import type {CosmeticCatalogEntry} from '@/lib/api/cosmeticPurchases';

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
  useQuery: ({queryKey, enabled}: {queryKey: unknown[]; enabled?: boolean}) => {
    // `enabled: false` is a query that never reaches the network — the request
    // budget assertions below depend on it not being recorded as a read.
    if (enabled === false) return {data: undefined, isLoading: false};
    mocks.query(queryKey);
    if (queryKey[0] === 'player') return {data: mocks.state.player, isLoading: false};
    if (queryKey[1] === 'cosmetic-catalog') return {data: mocks.state.catalog, isLoading: false};
    return {data: undefined, isLoading: false};
  },
  useQueryClient: () => ({setQueryData: mocks.setQueryData, invalidateQueries: mocks.invalidateQueries}),
  useMutation: ({onSuccess, onError}: {
    onSuccess?: (data: unknown, input: unknown) => void;
    onError?: (error: unknown, input: unknown) => void;
  }) => ({
    mutate: (input: unknown) => {
      mocks.mutate(input);
      if (mocks.saveShouldFail) {
        onError?.(new Error('rejected'), input);
        return;
      }
      onSuccess?.({...(mocks.state.player as object), ...(input as object)}, input);
    },
    isPending: false,
  }),
}));
vi.mock('@/lib/auth/oauth', () => ({logout: mocks.logout, endSession: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notify}));
vi.mock('@/lib/avatar', () => ({uploadAvatar: vi.fn(), deleteAvatar: vi.fn()}));
vi.mock('next/image', () => ({
  default: (props: {src: string; alt: string}) => <span role="img" aria-label={props.alt} data-src={props.src}/>,
}));
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

/** The deck picker arrives as its own chunk, so every deck assertion waits for
 * it rather than reading the loading placeholder. */
function deckTrigger() {
  return screen.findByRole('combobox', {name: 'Baralho'});
}

describe('ProfileMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.player = player;
    mocks.state.catalog = [];
    mocks.saveShouldFail = false;
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

  // Request budget: the popover is mounted on every authenticated page, so the
  // deck catalog (and the picker's chunk with it) must not be spent by a player
  // who never opens the menu.
  test('does not read the deck catalog until the menu is opened', async () => {
    render(<ProfileMenu/>);
    expect(mocks.query).toHaveBeenCalledWith(['player', 'me']);
    expect(mocks.query).not.toHaveBeenCalledWith(['wallet', 'cosmetic-catalog', 'deck']);

    await openProfile();
    await deckTrigger();
    expect(mocks.query).toHaveBeenCalledWith(['wallet', 'cosmetic-catalog', 'deck']);
  });

  // The route is the superset, and the popover keeps a link to it — but the
  // three quick edits stay here too.
  test('keeps the quick editors and still points at the full profile route', async () => {
    render(<ProfileMenu/>);
    await openProfile();

    expect(screen.getByRole('button', {name: /Editar perfil/})).toHaveAttribute('href', '/player-profile');
    expect(screen.getByRole('button', {name: 'Ana Silva'})).toBeInTheDocument();
    expect(screen.getByLabelText('Selecionar foto de perfil')).toBeInTheDocument();
    expect(await deckTrigger()).toBeInTheDocument();
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

  // The wallet-mode switch is the one preference that stayed on the route:
  // it needs the explanation a 360px panel cannot give it.
  test('never offers the wallet-mode switch, even with real money on', async () => {
    mocks.realMoney.enabled = true;
    render(<ProfileMenu/>);
    await openProfile();
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    expect(screen.queryByText('Modo de jogo')).not.toBeInTheDocument();
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
    expect(mocks.notify).toHaveBeenCalledWith('Agora você joga como Nova Ana.', 'info');
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
    // Escape belonged to the editor: the menu itself is still open.
    expect(screen.getByText('Nome de exibição')).toBeInTheDocument();
    expect(mocks.mutate).not.toHaveBeenCalled();
  });

  test('an empty draft is not a save, and Enter keeps the editor open', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Ana Silva'}));
    const input = screen.getByRole('textbox', {name: 'Nome de exibição'});
    await userEvent.clear(input);
    await userEvent.type(input, '   {Enter}');
    expect(mocks.mutate).not.toHaveBeenCalled();
    expect(screen.getByRole('textbox', {name: 'Nome de exibição'})).toBeInTheDocument();
  });

  test('saves the display name straight from the Enter key', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Ana Silva'}));
    await userEvent.type(screen.getByRole('textbox', {name: 'Nome de exibição'}), '{Enter}');
    expect(mocks.mutate).toHaveBeenCalledWith({name: 'Ana Silva'});
    expect(mocks.notify).toHaveBeenCalledWith('Agora você joga como Ana Silva.', 'info');
  });

  test('uploads a chosen profile photo and lets an existing one be removed', async () => {
    mocks.state.player = {...player, avatar_url: '/avatars/player-1.jpg'};
    render(<ProfileMenu/>);
    await openProfile();

    const file = new File(['photo'], 'photo.png', {type: 'image/png'});
    await userEvent.upload(screen.getByLabelText('Selecionar foto de perfil'), file);
    expect(mocks.mutate).toHaveBeenCalledWith(file);
    expect(mocks.notify).toHaveBeenCalledWith('Foto de perfil atualizada.', 'info');

    expect(screen.getByRole('button', {name: 'Trocar foto de perfil'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Remover foto de perfil'}));
    expect(mocks.notify).toHaveBeenCalledWith('Foto de perfil removida.', 'info');
    expect(mocks.setQueryData).toHaveBeenCalledWith(['player', 'me'], expect.any(Object));
  });

  test('the camera opens the file picker, and closing it empty uploads nothing', async () => {
    const click = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(() => {});
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(screen.getByRole('button', {name: 'Adicionar foto de perfil'}));
    expect(click).toHaveBeenCalled();
    click.mockRestore();

    fireEvent.change(screen.getByLabelText('Selecionar foto de perfil'), {target: {files: []}});
    expect(mocks.mutate).not.toHaveBeenCalled();
  });

  test('reports a photo the server would not take', async () => {
    mocks.saveShouldFail = true;
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.upload(screen.getByLabelText('Selecionar foto de perfil'),
      new File(['x'], 'x.png', {type: 'image/png'}));
    expect(mocks.notify).toHaveBeenCalledWith('Não foi possível atualizar a foto. Tente outra imagem.');
  });

  test('offers to add a photo, and no removal, when the player has none', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    expect(screen.getByRole('button', {name: 'Adicionar foto de perfil'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Remover foto de perfil'})).not.toBeInTheDocument();
  });

  test('switches the deck and confirms it for the next hand', async () => {
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(await deckTrigger());
    await userEvent.click(await screen.findByRole('option', {name: /Clássico/}));
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'two-color'});
    expect(mocks.notify).toHaveBeenCalledWith('Baralho pronto para a próxima mão.', 'info');
  });

  // Only a rejected wallet-mode change re-syncs the profile and explains
  // itself; a rejected deck change leaves the generic API toast to speak.
  test('a rejected deck change says nothing about the wallet mode', async () => {
    mocks.saveShouldFail = true;
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(await deckTrigger());
    await userEvent.click(await screen.findByRole('option', {name: /Clássico/}));
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'two-color'});
    expect(mocks.invalidateQueries).not.toHaveBeenCalled();
    expect(mocks.notify).not.toHaveBeenCalled();
  });

  test('locks an unowned premium deck with a link to the store instead of selecting it', async () => {
    mocks.state.catalog = [{kind: 'deck', id: 'golden', premium: true, owned: false, price_fichas: 500_000}];
    render(<ProfileMenu/>);
    await openProfile();
    await userEvent.click(await deckTrigger());

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
    await userEvent.click(await deckTrigger());

    const golden = await screen.findByRole('option', {name: 'Dourado'});
    expect(golden.tagName).not.toBe('A');
    await userEvent.click(golden);
    expect(mocks.mutate).toHaveBeenCalledWith({deck_variant: 'golden'});
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
    expect(screen.getByRole('button', {name: /Definir nome/})).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*0,00/)).toBeInTheDocument();
    expect(screen.getByText('Vitrine privada')).toBeInTheDocument();
  });
});
