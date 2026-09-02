import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ShareHandDialog} from './ShareHandDialog';
import {setPersistedHandShare} from '@/lib/handShareStorage';

const createHandShare = vi.fn();
const revokeHandShare = vi.fn();
vi.mock('@/lib/api/handShares', () => ({
  createHandShare: (...args: unknown[]) => createHandShare(...args),
  revokeHandShare: (...args: unknown[]) => revokeHandShare(...args),
}));

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  createHandShare.mockResolvedValue({token: 'token with spaces', expires_at: Date.now() + 60_000});
  revokeHandShare.mockResolvedValue(undefined);
});

describe('ShareHandDialog', () => {
  test('defaults losses to bad beat and submits the selected privacy options', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="hand/1" outcome="lost"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    
    expect(screen.getByRole('radio', {name: /Bad beat/})).toBeChecked();
    await user.click(screen.getByRole('radio', {name: /Brag/}));
    await user.click(screen.getByRole('checkbox', {name: /Mostrar minhas cartas/}));
    await user.selectOptions(screen.getByRole('combobox'), '30');
    await user.click(screen.getByRole('button', {name: /Criar link/}));
    
    await waitFor(() => expect(createHandShare).toHaveBeenCalledWith('hand/1', {
      kind: 'brag',
      include_hero_cards: false,
      expiry_days: 30,
    }, 'sandbox'));
    expect(screen.getByLabelText('Link compartilhável')).toHaveValue(
      `${window.location.origin}/share?id=token%20with%20spaces`,
    );
  });
  
  test('copies a created URL and exposes visual confirmation', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h2" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    expect(screen.getByRole('radio', {name: /Brag/})).toBeChecked();
    await user.click(screen.getByRole('radio', {name: /Bad beat/}));
    expect(screen.getByRole('radio', {name: /Bad beat/})).toBeChecked();
    await user.click(screen.getByRole('radio', {name: /Brag/}));
    await user.click(screen.getByRole('button', {name: /Criar link/}));
    const url = await screen.findByLabelText('Link compartilhável');
    await user.click(url);
    const writeText = vi.spyOn(navigator.clipboard, 'writeText');
    await user.click(screen.getByRole('button', {name: /Copiar/}));
    expect(writeText).toHaveBeenCalledWith((url as HTMLInputElement).value);
    expect(screen.getByRole('button', {name: /Copiado/})).toBeInTheDocument();
  });
  
  test('prevents duplicate creation while the request is pending', async () => {
    let resolve!: (share: { token: string }) => void;
    createHandShare.mockReturnValue(new Promise(resolvePromise => {
      resolve = resolvePromise;
    }));
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h3" outcome="tied"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    const createButton = screen.getByRole('button', {name: /Criar link/});
    await user.click(createButton);
    expect(createButton).toBeDisabled();
    await user.click(createButton);
    expect(createHandShare).toHaveBeenCalledOnce();
    
    resolve({token: 'eventual'});
    expect(await screen.findByLabelText('Link compartilhável')).toBeInTheDocument();
  });

  test('keeps creation failures recoverable', async () => {
    createHandShare.mockRejectedValueOnce(new Error('offline'));
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h4" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));

    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível criar o link');
    expect(screen.getByRole('button', {name: 'Criar link'})).toBeEnabled();
  });

  test('offers manual copying when clipboard access fails', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h5" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));
    await screen.findByLabelText('Link compartilhável');
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValueOnce(new Error('denied'));
    await user.click(screen.getByRole('button', {name: 'Copiar'}));

    expect(await screen.findByRole('alert')).toHaveTextContent('Selecione o link e copie manualmente');
    expect(screen.getByLabelText('Link compartilhável')).toBeInTheDocument();
  });

  test('reopening for an already-shared hand shows the existing link without re-creating', async () => {
    setPersistedHandShare('h6', {token: 'existing-token', expiresAt: Date.now() + 60_000});
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h6" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));

    expect(screen.getByLabelText('Link compartilhável')).toHaveValue(
      `${window.location.origin}/share?id=existing-token`,
    );
    expect(screen.getByText('Você já tem um link para esta mão.')).toBeInTheDocument();
    expect(createHandShare).not.toHaveBeenCalled();
    expect(screen.getByRole('button', {name: /Revogar/})).toBeInTheDocument();
  });

  test('revoking clears the persisted link and the dialog returns to its pre-share state', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h7" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));
    await screen.findByLabelText('Link compartilhável');

    await user.click(screen.getByRole('button', {name: /Revogar/}));

    expect(revokeHandShare).toHaveBeenCalledWith('token with spaces');
    await waitFor(() => expect(screen.queryByLabelText('Link compartilhável')).not.toBeInTheDocument());
    expect(screen.getByText(/Link revogado\./)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Criar link/})).toBeInTheDocument();
  });

  test('a fresh mount after a revoke has nothing persisted for that hand', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h7b" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));
    await screen.findByLabelText('Link compartilhável');
    await user.click(screen.getByRole('button', {name: /Revogar/}));
    await waitFor(() => expect(revokeHandShare).toHaveBeenCalled());

    render(<ShareHandDialog handId="h7b" outcome="won"/>);
    const shareButtons = screen.getAllByRole('button', {name: 'Compartilhar'});
    await user.click(shareButtons[shareButtons.length - 1]);
    expect(screen.getAllByRole('radio', {name: /Brag/}).length).toBeGreaterThan(0);
  });

  test('shows an inline error and keeps the link when revoke fails', async () => {
    revokeHandShare.mockRejectedValueOnce(new Error('offline'));
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h8" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));
    const url = await screen.findByLabelText('Link compartilhável');

    await user.click(screen.getByRole('button', {name: /Revogar/}));

    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível revogar o link');
    expect(url).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Revogar/})).toBeEnabled();
  });

  test('disables the revoke button while the request is pending', async () => {
    let resolveRevoke!: () => void;
    revokeHandShare.mockReturnValue(new Promise<void>(resolvePromise => {
      resolveRevoke = resolvePromise;
    }));
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h9" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    await user.click(screen.getByRole('button', {name: 'Criar link'}));
    await screen.findByLabelText('Link compartilhável');
    const revokeButton = screen.getByRole('button', {name: /Revogar/});
    await user.click(revokeButton);
    expect(revokeButton).toBeDisabled();
    await user.click(revokeButton);
    expect(revokeHandShare).toHaveBeenCalledOnce();

    resolveRevoke();
    await waitFor(() => expect(screen.getByText(/Link revogado\./)).toBeInTheDocument());
  });
});
