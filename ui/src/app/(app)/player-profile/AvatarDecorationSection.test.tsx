import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {AvatarDecorationSection} from './AvatarDecorationSection';
import type {PlayerProfile} from '@/lib/api/player';
import {PLAYER_ME_KEY} from '@/lib/hooks/useProfileEdits';

const {listOwnedAvatarCosmetics, updateMe, notify} = vi.hoisted(() => ({
  listOwnedAvatarCosmetics: vi.fn(), updateMe: vi.fn(), notify: vi.fn(),
}));
vi.mock('@/lib/avatarCosmetics', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/avatarCosmetics')>(),
  listOwnedAvatarCosmetics,
}));
vi.mock('@/lib/api/player', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/player')>(),
  updateMe,
}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));
// The preview's contract is "the same PlayerAvatar, fed the pending ids"; the
// art itself is PlayerAvatar's own suite.
vi.mock('@/components/ui/player-avatar', () => ({
  PlayerAvatar: (p: { size?: number; frameId?: string; hatId?: string; accessoryId?: string; badgeIds?: string[] }) =>
    <span data-testid={`preview-${p.size}`} data-frame={p.frameId ?? ''} data-hat={p.hatId ?? ''}
          data-accessory={p.accessoryId ?? ''} data-badges={(p.badgeIds ?? []).join(',')}/>,
}));

const me: PlayerProfile = {
  user_id: 'p1', wallet_mode: 'sandbox', poker_terms_accepted: true, showcase_public: false,
  table_public: false, playstyle_public: false, name: 'Ana',
};

const OWNED: Record<string, string[]> = {
  frame: ['season-2026-q3', 'chip-ring'],
  hat: ['crown', 'top-hat'],
  accessory: ['monocle'],
  badge: ['season-2026-q3-top10', 'season-2026-q3-champion'],
};

const wrapper = ({children}: { children: ReactNode }) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

const group = (name: string) => screen.findByRole('radiogroup', {name});
const saveButton = () => screen.getByRole('button', {name: 'Salvar visual do avatar'});

describe('AvatarDecorationSection', () => {
  beforeEach(() => {
    listOwnedAvatarCosmetics.mockReset();
    updateMe.mockReset();
    notify.mockReset();
    listOwnedAvatarCosmetics.mockImplementation((slot: string) => Promise.resolve(OWNED[slot]));
  });

  test('shows a short empty line per slot when nothing is owned', async () => {
    listOwnedAvatarCosmetics.mockResolvedValue([]);
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    expect(await screen.findAllByText(/Você ainda não tem itens deste tipo/)).toHaveLength(4);
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  });

  test('lists only owned ids per slot, plus Nenhum/Nenhuma, with radio/checkbox semantics', async () => {
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    for (const slot of ['frame', 'hat', 'accessory', 'badge']) {
      expect(listOwnedAvatarCosmetics).toHaveBeenCalledWith(slot);
    }
    const hats = within(await group('Chapéu equipado')).getAllByRole('radio').map(r => r.closest('label')?.textContent?.trim());
    expect(hats).toEqual(['Nenhum', 'Coroa', 'Cartola']);
    const frames = within(await group('Borda equipada')).getAllByRole('radio');
    expect(frames).toHaveLength(3);
    expect(frames[0].closest('label')?.textContent?.trim()).toBe('Nenhuma');
    expect(within(await group('Acessório equipado')).getByRole('radio', {name: 'Monóculo'})).toBeInTheDocument();
    expect(screen.queryByRole('radio', {name: 'Gorro'})).not.toBeInTheDocument();
    expect(await screen.findByRole('checkbox', {name: /Top 10 da Temporada/})).toBeInTheDocument();
    expect(screen.getAllByRole('checkbox')).toHaveLength(2);
    expect(within(await group('Chapéu equipado')).getByRole('radio', {name: 'Nenhum'})).toBeChecked();
  });

  test('an owned id the registry does not know lists under its raw id and does not crash', async () => {
    listOwnedAvatarCosmetics.mockImplementation((slot: string) =>
      Promise.resolve(slot === 'hat' ? ['future-hat'] : []));
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    const option = await screen.findByRole('radio', {name: 'future-hat'});
    expect(option.closest('label')?.querySelector('svg')).toBeNull();
    await userEvent.click(option);
    expect(screen.getByTestId('preview-72')).toHaveAttribute('data-hat', 'future-hat');
  });

  test.each([
    ['Borda equipada', 'Nenhuma', 'equipped_frame_id', {equipped_frame_id: 'chip-ring'}],
    ['Chapéu equipado', 'Nenhum', 'equipped_hat_id', {equipped_hat_id: 'crown'}],
    ['Acessório equipado', 'Nenhum', 'equipped_accessory_id', {equipped_accessory_id: 'monocle'}],
  ] as const)('the empty option in %s (%s) unequips only that slot', async (name, none, field, equipped) => {
    const current = {...me, equipped_frame_id: 'season-2026-q3', equipped_hat_id: 'top-hat',
      equipped_accessory_id: 'monocle', equipped_badge_ids: ['season-2026-q3-top10'], ...equipped};
    updateMe.mockResolvedValue({...current, [field]: ''});
    render(<AvatarDecorationSection me={current}/>, {wrapper});
    await userEvent.click(within(await group(name)).getByRole('radio', {name: none}));
    await userEvent.click(saveButton());
    expect(updateMe).toHaveBeenCalledWith({[field]: ''});
  });

  test('a saved id no longer owned stays listed and checked, so the picker matches the preview', async () => {
    render(<AvatarDecorationSection me={{...me, equipped_hat_id: 'beanie', equipped_frame_id: 'gone-frame',
      equipped_badge_ids: ['veteran']}}/>, {wrapper});
    const hats = within(await group('Chapéu equipado')).getAllByRole('radio').map(r => r.closest('label')?.textContent?.trim());
    expect(hats).toEqual(['Nenhum', 'Coroa', 'Cartola', 'Gorro']);
    expect(screen.getByRole('radio', {name: 'Gorro'})).toBeChecked();
    expect(within(await group('Borda equipada')).getByRole('radio', {name: 'gone-frame'})).toBeChecked();
    expect(await screen.findByRole('checkbox', {name: /Veterano/})).toBeChecked();
    expect(saveButton()).toBeDisabled();
  });

  test('blocks a fourth badge with the existing message', async () => {
    listOwnedAvatarCosmetics.mockImplementation((slot: string) =>
      Promise.resolve(slot === 'badge' ? ['a', 'b', 'c', 'd'] : []));
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    for (const id of ['a', 'b', 'c']) {
      await userEvent.click(await screen.findByRole('checkbox', {name: id}));
    }
    await userEvent.click(screen.getByRole('checkbox', {name: 'd'}));
    expect(notify).toHaveBeenCalledWith('Escolha no máximo 3 emblemas.', 'info');
    expect(screen.getByRole('checkbox', {name: 'd'})).not.toBeChecked();
    expect(screen.getByText('3/3')).toBeInTheDocument();
  });

  test('both previews (72px and seat 36px) follow the pending selection', async () => {
    render(<AvatarDecorationSection me={{...me, equipped_hat_id: 'crown'}}/>, {wrapper});
    for (const size of [72, 36]) expect(screen.getByTestId(`preview-${size}`)).toHaveAttribute('data-hat', 'crown');
    await userEvent.click(await screen.findByRole('radio', {name: 'Borda de Fichas'}));
    await userEvent.click(screen.getByRole('radio', {name: 'Cartola'}));
    await userEvent.click(screen.getByRole('radio', {name: 'Monóculo'}));
    await userEvent.click(screen.getByRole('checkbox', {name: /Campeão da Temporada/}));
    for (const size of [72, 36]) {
      const preview = screen.getByTestId(`preview-${size}`);
      expect(preview).toHaveAttribute('data-frame', 'chip-ring');
      expect(preview).toHaveAttribute('data-hat', 'top-hat');
      expect(preview).toHaveAttribute('data-accessory', 'monocle');
      expect(preview).toHaveAttribute('data-badges', 'season-2026-q3-champion');
    }
  });

  test('save is disabled until something changes, ignores badge order, and sends only changed fields', async () => {
    const current = {...me, equipped_frame_id: 'season-2026-q3',
      equipped_badge_ids: ['season-2026-q3-top10', 'season-2026-q3-champion']};
    const saved = {...current, equipped_hat_id: 'crown'};
    updateMe.mockResolvedValue(saved);
    render(<AvatarDecorationSection me={current}/>, {wrapper});
    await group('Chapéu equipado');
    expect(saveButton()).toBeDisabled();

    // Re-ordering the same badges is not a change.
    await userEvent.click(screen.getByRole('checkbox', {name: /Top 10 da Temporada/}));
    await userEvent.click(screen.getByRole('checkbox', {name: /Top 10 da Temporada/}));
    expect(saveButton()).toBeDisabled();

    await userEvent.click(screen.getByRole('radio', {name: 'Coroa'}));
    expect(saveButton()).not.toBeDisabled();
    await userEvent.click(saveButton());
    expect(updateMe).toHaveBeenCalledTimes(1);
    expect(updateMe).toHaveBeenCalledWith({equipped_hat_id: 'crown'});
    await vi.waitFor(() => expect(notify).toHaveBeenCalledWith('Visual do avatar atualizado.', 'info'));
  });

  test('sends changed badges as the full new list', async () => {
    updateMe.mockResolvedValue(me);
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    await userEvent.click(await screen.findByRole('checkbox', {name: /Top 10 da Temporada/}));
    await userEvent.click(saveButton());
    expect(updateMe).toHaveBeenCalledWith({equipped_badge_ids: ['season-2026-q3-top10']});
  });

  test('a failed save reports the error', async () => {
    updateMe.mockRejectedValue(new Error('boom'));
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    await userEvent.click(await screen.findByRole('radio', {name: 'Coroa'}));
    await userEvent.click(saveButton());
    await vi.waitFor(() => expect(notify).toHaveBeenCalledWith('Não foi possível salvar agora. Tente novamente.', 'error'));
  });

  test('a failed save refetches the profile so the picker resyncs', async () => {
    updateMe.mockRejectedValue(new Error('boom'));
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    render(<AvatarDecorationSection me={me}/>, {
      wrapper: ({children}: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>,
    });
    await userEvent.click(await screen.findByRole('radio', {name: 'Coroa'}));
    await userEvent.click(saveButton());
    await vi.waitFor(() => expect(invalidate).toHaveBeenCalledWith({queryKey: PLAYER_ME_KEY}));
  });

  test('a failed owned read offers a retry', async () => {
    listOwnedAvatarCosmetics.mockImplementation((slot: string) =>
      slot === 'hat' ? Promise.reject(new Error('down')) : Promise.resolve([]));
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    expect(await screen.findByText(/Não foi possível carregar seus chapéus agora/)).toBeInTheDocument();
    listOwnedAvatarCosmetics.mockImplementation((slot: string) => Promise.resolve(slot === 'hat' ? ['crown'] : []));
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(await screen.findByRole('radio', {name: 'Coroa'})).toBeInTheDocument();
  });

  test('the section keeps its heading wiring', async () => {
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    expect(screen.getByRole('region', {name: 'Visual do avatar'})).toBeInTheDocument();
    await group('Borda equipada');
  });
});
