import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {MyHandSharesPanel} from './MyHandSharesPanel';
import type {HandShareSummary} from '@/lib/api/handShares';
import {getPersistedHandShare, setPersistedHandShare} from '@/lib/handShareStorage';

const {listMyHandShares, revokeHandShare} = vi.hoisted(() => ({
  listMyHandShares: vi.fn(), revokeHandShare: vi.fn(),
}));
vi.mock('@/lib/api/handShares', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/handShares')>(), listMyHandShares, revokeHandShare,
}));

const wrapper = ({children}: {children: ReactNode}) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}, mutations: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

const share = (overrides: Partial<HandShareSummary> = {}): HandShareSummary => ({
  token: 'tok-1', kind: 'brag', outcome: 'won', net_change: 1200,
  created_at: Date.now() - 3600_000, expires_at: Date.now() + 6 * 24 * 3600_000, ...overrides,
});

describe('MyHandSharesPanel (#96)', () => {
  beforeEach(() => {
    listMyHandShares.mockReset();
    revokeHandShare.mockReset();
    window.localStorage.clear();
  });

  test('lists every live share the player created, with its kind and dates', async () => {
    listMyHandShares.mockResolvedValue([
      share({token: 'tok-brag', kind: 'brag', outcome: 'won', net_change: 1200}),
      share({token: 'tok-bad', kind: 'bad_beat', outcome: 'lost', net_change: -800}),
    ]);
    render(<MyHandSharesPanel/>, {wrapper});

    expect(await screen.findByText('Brag')).toBeInTheDocument();
    expect(screen.getByText('Bad beat')).toBeInTheDocument();
    expect(screen.getByText('+1.200')).toBeInTheDocument();
    expect(screen.getByText('-800')).toBeInTheDocument();
    expect(screen.getAllByRole('button', {name: 'Revogar'})).toHaveLength(2);
    expect(screen.getAllByText(/^Expira /)).toHaveLength(2);
  });

  test('revoking calls the API for that token, refetches, and forgets the local link', async () => {
    const user = userEvent.setup();
    setPersistedHandShare('hand-9', {token: 'tok-bad', expiresAt: Date.now() + 60_000});
    listMyHandShares
      .mockResolvedValueOnce([share({token: 'tok-keep'}), share({token: 'tok-bad', kind: 'bad_beat'})])
      .mockResolvedValue([share({token: 'tok-keep'})]);
    revokeHandShare.mockResolvedValue(undefined);
    render(<MyHandSharesPanel/>, {wrapper});

    const rows = await screen.findAllByRole('listitem');
    expect(rows).toHaveLength(2);
    await user.click(screen.getAllByRole('button', {name: 'Revogar'})[1]);

    await waitFor(() => expect(revokeHandShare).toHaveBeenCalledWith('tok-bad'));
    // The list is server state, so the row disappears via a refetch, not a
    // local splice — that is what makes /share?id= answer "revogado" at once.
    await waitFor(() => expect(screen.getAllByRole('listitem')).toHaveLength(1));
    // And the dialog's per-hand memory of that token is dropped, so reopening
    // the hand offers a fresh link instead of one the server has forgotten.
    expect(getPersistedHandShare('hand-9')).toBeNull();
  });

  test('a failed revocation keeps the row and says so, without clearing local state', async () => {
    const user = userEvent.setup();
    setPersistedHandShare('hand-9', {token: 'tok-1', expiresAt: Date.now() + 60_000});
    listMyHandShares.mockResolvedValue([share()]);
    revokeHandShare.mockRejectedValue(new Error('boom'));
    render(<MyHandSharesPanel/>, {wrapper});

    await user.click(await screen.findByRole('button', {name: 'Revogar'}));

    expect(await screen.findByText(/Não foi possível revogar o link agora/)).toBeInTheDocument();
    expect(screen.getAllByRole('listitem')).toHaveLength(1);
    expect(getPersistedHandShare('hand-9')?.token).toBe('tok-1');
  });

  test('teaches the feature when the player has no links yet', async () => {
    listMyHandShares.mockResolvedValue([]);
    render(<MyHandSharesPanel/>, {wrapper});
    expect(await screen.findByText('Nenhum link ativo')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Revogar'})).not.toBeInTheDocument();
  });

  test('a failed listing is recoverable and does not claim the links are gone', async () => {
    const user = userEvent.setup();
    listMyHandShares.mockRejectedValueOnce(new Error('offline')).mockResolvedValue([share()]);
    render(<MyHandSharesPanel/>, {wrapper});

    expect(await screen.findByText('Seus links não abriram desta vez')).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(await screen.findByRole('button', {name: 'Revogar'})).toBeInTheDocument();
  });

  test('copies the public share URL for a row', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
    listMyHandShares.mockResolvedValue([share({token: 'token with spaces'})]);
    render(<MyHandSharesPanel/>, {wrapper});

    await user.click(await screen.findByRole('button', {name: 'Copiar link'}));
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/share?id=token%20with%20spaces`);
    expect(await screen.findByRole('button', {name: 'Copiado'})).toBeInTheDocument();
  });

  test('a clipboard the browser refuses leaves the button un-confirmed', async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: undefined});
    listMyHandShares.mockResolvedValue([share()]);
    render(<MyHandSharesPanel/>, {wrapper});

    await user.click(await screen.findByRole('button', {name: 'Copiar link'}));
    expect(screen.getByRole('button', {name: 'Copiar link'})).toBeInTheDocument();
  });
});
