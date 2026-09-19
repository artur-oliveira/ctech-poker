import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {AvatarDecorationSection} from './AvatarDecorationSection';
import type {PlayerProfile} from '@/lib/api/player';

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

const me: PlayerProfile = {
  user_id: 'p1', wallet_mode: 'sandbox', poker_terms_accepted: true, showcase_public: false,
  table_public: false, playstyle_public: false, name: 'Ana',
};

const wrapper = ({children}: { children: ReactNode }) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe('AvatarDecorationSection', () => {
  beforeEach(() => {
    listOwnedAvatarCosmetics.mockReset();
    updateMe.mockReset();
    notify.mockReset();
    listOwnedAvatarCosmetics.mockImplementation((kind: string) =>
      Promise.resolve(kind === 'frame' ? ['season-2026-q3'] : ['season-2026-q3-top10', 'season-2026-q3-champion']));
  });

  test('falls back to empty copy with nothing owned', async () => {
    listOwnedAvatarCosmetics.mockResolvedValue([]);
    render(<AvatarDecorationSection me={me}/>, {wrapper});
    expect(await screen.findByText(/Nenhuma moldura desbloqueada ainda/)).toBeInTheDocument();
    expect(await screen.findByText(/Nenhum emblema desbloqueado ainda/)).toBeInTheDocument();
  });

  test('lists only owned frames/badges and saves the chosen equip set', async () => {
    updateMe.mockResolvedValue({...me, equipped_frame_id: 'season-2026-q3', equipped_badge_ids: ['season-2026-q3-top10']});
    render(<AvatarDecorationSection me={me}/>, {wrapper});

    const frameOption = await screen.findByRole('radio', {name: /Moldura da Temporada/});
    expect(screen.getByRole('button', {name: 'Salvar moldura e emblemas'})).toBeDisabled();

    await userEvent.click(frameOption);
    await userEvent.click(screen.getByRole('checkbox', {name: /Top 10 da Temporada/}));
    const saveButton = screen.getByRole('button', {name: 'Salvar moldura e emblemas'});
    expect(saveButton).not.toBeDisabled();
    await userEvent.click(saveButton);

    expect(updateMe).toHaveBeenCalledWith({
      equipped_frame_id: 'season-2026-q3', equipped_badge_ids: ['season-2026-q3-top10']
    });
    await vi.waitFor(() => expect(notify).toHaveBeenCalledWith('Moldura e emblemas atualizados.', 'info'));
  });

  test('caps equipped badges at three', async () => {
    listOwnedAvatarCosmetics.mockImplementation((kind: string) =>
      Promise.resolve(kind === 'frame' ? [] : ['a', 'b', 'c', 'd']));
    render(<AvatarDecorationSection me={me}/>, {wrapper});

    for (const id of ['a', 'b', 'c']) {
      await userEvent.click(await screen.findByRole('checkbox', {name: id}));
    }
    await userEvent.click(screen.getByRole('checkbox', {name: 'd'}));
    expect(notify).toHaveBeenCalledWith('Escolha no máximo 3 emblemas.', 'info');
    expect(screen.getByRole('checkbox', {name: 'd'})).not.toBeChecked();
  });

  test('unequipping a frame needs no ownership and is always available', async () => {
    render(<AvatarDecorationSection me={{...me, equipped_frame_id: 'season-2026-q3'}}/>, {wrapper});
    await userEvent.click(await screen.findByRole('radio', {name: 'Nenhuma'}));
    expect(screen.getByRole('button', {name: 'Salvar moldura e emblemas'})).not.toBeDisabled();
  });
});
