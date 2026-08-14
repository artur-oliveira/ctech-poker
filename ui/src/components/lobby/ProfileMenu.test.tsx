import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileMenu} from './ProfileMenu';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutate: vi.fn(),
  setQueryData: vi.fn(),
  logout: vi.fn(),
  notify: vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({setQueryData: mocks.setQueryData}),
  useMutation: ({onSuccess}: { onSuccess: (data: unknown, input: unknown) => void }) => ({
    mutate: (input: unknown) => {
      mocks.mutate(input);
      onSuccess({...mocks.query().data, ...(input as object)}, input);
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
    mocks.query.mockReturnValue({data: player});
  });
  
  test('summarizes the active wallet and exposes both balances in the menu', async () => {
    render(<ProfileMenu/>);
    expect(screen.getByText('12.345 fichas')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: /Abrir loja/})).toHaveAttribute('href', '/store');
    expect(screen.getByText('AS')).toBeInTheDocument();
    
    await openProfile();
    expect(screen.getByText('Sandbox')).toBeInTheDocument();
    expect(screen.getAllByText('12.345 fichas')).toHaveLength(2);
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Loja/})).toHaveAttribute('href', '/store');
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
    mocks.query.mockReturnValue({data: {...player, wallet_mode: 'real'}});
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
    mocks.query.mockReturnValue({data: {...player, avatar_url: '/avatars/player-1.jpg'}});
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
    mocks.query.mockReturnValue({data: undefined});
    render(<ProfileMenu/>);
    expect(screen.getByText('0 fichas')).toBeInTheDocument();

    await openProfile();
    expect(screen.getByRole('button', {name: /Definir nome/})).toBeInTheDocument();
    expect(screen.getByText(/R\$\s*0,00/)).toBeInTheDocument();
    expect(screen.getByText('Vitrine privada')).toBeInTheDocument();
  });

  test('formats the real-money wallet in the collapsed summary', () => {
    mocks.query.mockReturnValue({data: {...player, wallet_mode: 'real'}});
    render(<ProfileMenu/>);
    expect(screen.getByText(/R\$\s*987,60/)).toBeInTheDocument();
  });
});
