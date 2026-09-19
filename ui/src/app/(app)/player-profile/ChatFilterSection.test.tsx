import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ChatFilterSection} from './ChatFilterSection';
import {CHAT_PREFS_MAX_WORDS} from '@/lib/api/chatPrefs';

const {getChatPrefs, saveChatPrefs, notify} = vi.hoisted(() => ({
  getChatPrefs: vi.fn(), saveChatPrefs: vi.fn(), notify: vi.fn(),
}));
vi.mock('@/lib/api/chatPrefs', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/chatPrefs')>(), getChatPrefs, saveChatPrefs,
}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));

const wrapper = ({children}: { children: ReactNode }) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}, mutations: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe('ChatFilterSection (#327)', () => {
  beforeEach(() => {
    getChatPrefs.mockReset();
    saveChatPrefs.mockReset();
    notify.mockReset();
  });

  test('explains the floor stays in place when the player has no personal words yet', async () => {
    getChatPrefs.mockResolvedValue({extra_words: []});
    render(<ChatFilterSection/>, {wrapper});
    expect(await screen.findByText(/ainda não adicionou/)).toBeInTheDocument();
    // The copy must never suggest the floor itself can be removed here.
    expect(screen.getByText(/nunca remove ou enfraquece/)).toBeInTheDocument();
  });

  test('lists the player\'s own extra words and lets them add one', async () => {
    getChatPrefs.mockResolvedValue({extra_words: ['chato']});
    saveChatPrefs.mockResolvedValue({extra_words: ['chato', 'lento']});
    const user = userEvent.setup();
    render(<ChatFilterSection/>, {wrapper});

    expect(await screen.findByText('chato')).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText('Adicionar palavra…'), 'Lento');
    await user.click(screen.getByRole('button', {name: 'Adicionar'}));

    await waitFor(() => expect(saveChatPrefs).toHaveBeenCalledWith(['chato', 'lento'], expect.anything()));
    expect(await screen.findByText('lento')).toBeInTheDocument();
  });

  test('removing a word saves the remaining list, never clearing the floor server-side', async () => {
    getChatPrefs.mockResolvedValue({extra_words: ['chato', 'lento']});
    saveChatPrefs.mockResolvedValue({extra_words: ['lento']});
    const user = userEvent.setup();
    render(<ChatFilterSection/>, {wrapper});

    await screen.findByText('chato');
    await user.click(screen.getByRole('button', {name: 'Remover "chato" do seu filtro'}));

    await waitFor(() => expect(saveChatPrefs).toHaveBeenCalledWith(['lento'], expect.anything()));
  });

  test('rejects adding past the word limit without ever calling save', async () => {
    getChatPrefs.mockResolvedValue({extra_words: Array.from({length: CHAT_PREFS_MAX_WORDS}, (_, i) => `w${i}`)});
    const user = userEvent.setup();
    render(<ChatFilterSection/>, {wrapper});

    await screen.findByText('w0');
    await user.type(screen.getByPlaceholderText('Adicionar palavra…'), 'novapalavra');
    await user.click(screen.getByRole('button', {name: 'Adicionar'}));

    expect(await screen.findByRole('alert')).toHaveTextContent(`até ${CHAT_PREFS_MAX_WORDS} palavras`);
    expect(saveChatPrefs).not.toHaveBeenCalled();
  });

  test('reports a save failure without losing the word the player already had', async () => {
    getChatPrefs.mockResolvedValue({extra_words: ['chato']});
    saveChatPrefs.mockRejectedValue(new Error('boom'));
    const user = userEvent.setup();
    render(<ChatFilterSection/>, {wrapper});

    await user.type(await screen.findByPlaceholderText('Adicionar palavra…'), 'lento');
    await user.click(screen.getByRole('button', {name: 'Adicionar'}));

    await waitFor(() => expect(notify).toHaveBeenCalledWith(
      'Não foi possível salvar seu filtro de chat. Tente novamente.',
    ));
    // The cache was never optimistically updated, so the word list still
    // reflects what the server actually has.
    expect(screen.getByText('chato')).toBeInTheDocument();
  });

  test('shows a retry affordance when the load fails, without losing the section heading', async () => {
    getChatPrefs.mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({extra_words: ['chato']});
    const user = userEvent.setup();
    render(<ChatFilterSection/>, {wrapper});

    expect(await screen.findByRole('alert')).toHaveTextContent('não abriu');
    await user.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(await screen.findByText('chato')).toBeInTheDocument();
  });
});
