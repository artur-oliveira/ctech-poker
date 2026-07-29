import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ShareHandDialog} from './ShareHandDialog';

const createHandShare = vi.fn();
vi.mock('@/lib/api/handShares', () => ({
  createHandShare: (...args: unknown[]) => createHandShare(...args),
}));

beforeEach(() => {
  vi.clearAllMocks();
  createHandShare.mockResolvedValue({token: 'token with spaces'});
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
    }));
    expect(screen.getByLabelText('Link compartilhável')).toHaveValue(
      `${window.location.origin}/share?id=token%20with%20spaces`,
    );
  });
  
  test('copies a created URL and exposes visual confirmation', async () => {
    const user = userEvent.setup();
    render(<ShareHandDialog handId="h2" outcome="won"/>);
    await user.click(screen.getByRole('button', {name: 'Compartilhar'}));
    expect(screen.getByRole('radio', {name: /Brag/})).toBeChecked();
    await user.click(screen.getByRole('button', {name: /Criar link/}));
    const url = await screen.findByLabelText('Link compartilhável');
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
});
