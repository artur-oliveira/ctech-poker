import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {CosmeticLoadoutSection} from './CosmeticLoadoutSection';
import {ApiError} from '@/lib/api/client';
import type {CosmeticLoadout} from '@/lib/api/cosmeticLoadouts';

const {listCosmeticLoadouts, createCosmeticLoadout, deleteCosmeticLoadout, applyCosmeticLoadout, notify} = vi.hoisted(() => ({
  listCosmeticLoadouts: vi.fn(), createCosmeticLoadout: vi.fn(), deleteCosmeticLoadout: vi.fn(),
  applyCosmeticLoadout: vi.fn(), notify: vi.fn(),
}));
vi.mock('@/lib/api/cosmeticLoadouts', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/cosmeticLoadouts')>(),
  listCosmeticLoadouts, createCosmeticLoadout, deleteCosmeticLoadout, applyCosmeticLoadout,
}));
vi.mock('@/lib/notify', () => ({pushNotification: notify}));
vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

function loadout(overrides: Partial<CosmeticLoadout> = {}): CosmeticLoadout {
  return {
    id: 'loadout-1', name: 'Mesa de torneio', selections: {deck: 'four-color', felt: 'classic'},
    created_at: '2026-09-01T00:00:00Z', ...overrides,
  };
}

const wrapper = ({children}: { children: ReactNode }) => {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function renderSection(overrides: Partial<Parameters<typeof CosmeticLoadoutSection>[0]> = {}) {
  return render(<CosmeticLoadoutSection deckVariant="four-color" tableTheme="classic" seen
    sectionRef={() => {}} {...overrides}/>, {wrapper});
}

describe('CosmeticLoadoutSection', () => {
  beforeEach(() => {
    listCosmeticLoadouts.mockReset().mockResolvedValue([]);
    createCosmeticLoadout.mockReset();
    deleteCosmeticLoadout.mockReset();
    applyCosmeticLoadout.mockReset();
    notify.mockReset();
  });

  test('shows a loading skeleton while the list is not seen/pending', () => {
    listCosmeticLoadouts.mockReturnValue(new Promise(() => {}));
    renderSection();
    expect(screen.getByText('Carregando seus combos…')).toBeInTheDocument();
  });

  test('falls back to an empty state with no saved loadouts', async () => {
    renderSection();
    expect(await screen.findByText(/Nenhum combo salvo ainda/)).toBeInTheDocument();
  });

  test('offers a retry on error', async () => {
    listCosmeticLoadouts.mockRejectedValue(new Error('boom'));
    renderSection();
    expect(await screen.findByText('Não foi possível carregar seus combos agora.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(listCosmeticLoadouts).toHaveBeenCalledTimes(2);
  });

  test('saves the current combo under a name', async () => {
    createCosmeticLoadout.mockResolvedValue(loadout());
    renderSection();
    await screen.findByText(/Nenhum combo salvo ainda/);

    await userEvent.click(screen.getByRole('button', {name: 'Salvar como combo'}));
    await userEvent.type(screen.getByLabelText('Nome do combo'), 'Mesa de torneio');
    await userEvent.click(screen.getByRole('button', {name: 'Salvar combo'}));

    expect(createCosmeticLoadout).toHaveBeenCalledWith('Mesa de torneio', {deck: 'four-color', felt: 'classic'});
    await vi.waitFor(() => expect(notify).toHaveBeenCalledWith('Combo salvo. Ele já aparece na sua lista.', 'info'));
  });

  test('disables saving once the player is at the loadout limit', async () => {
    listCosmeticLoadouts.mockResolvedValue(
      Array.from({length: 5}, (_, index) => loadout({id: `loadout-${index}`, name: `Combo ${index}`}))
    );
    renderSection();
    expect(await screen.findByText(/já tem 5 combos salvos/)).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Salvar como combo'})).toBeDisabled();
  });

  test('applies a saved loadout after the preview confirm', async () => {
    listCosmeticLoadouts.mockResolvedValue([loadout()]);
    applyCosmeticLoadout.mockResolvedValue(loadout());
    renderSection();

    await userEvent.click(await screen.findByRole('button', {name: 'Aplicar'}));
    expect(screen.getByRole('heading', {name: 'Aplicar "Mesa de torneio"?'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Aplicar combo'}));

    expect(applyCosmeticLoadout).toHaveBeenCalledWith('loadout-1');
    expect(await screen.findByText('Pronto para a mesa')).toBeInTheDocument();
  });

  test('reports an unowned item cleanly when applying fails with 400', async () => {
    listCosmeticLoadouts.mockResolvedValue([loadout()]);
    applyCosmeticLoadout.mockRejectedValue(new ApiError('bad request', 400));
    renderSection();

    await userEvent.click(await screen.findByRole('button', {name: 'Aplicar'}));
    await userEvent.click(screen.getByRole('button', {name: 'Aplicar combo'}));

    expect(await screen.findByRole('alert')).toHaveTextContent(/não é mais seu/);
  });

  test('deletes a saved loadout after confirming', async () => {
    listCosmeticLoadouts.mockResolvedValue([loadout()]);
    deleteCosmeticLoadout.mockResolvedValue(undefined);
    renderSection();

    await userEvent.click(await screen.findByRole('button', {name: 'Excluir combo Mesa de torneio'}));
    expect(screen.getByRole('heading', {name: 'Excluir "Mesa de torneio"?'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Excluir combo'}));

    expect(deleteCosmeticLoadout).toHaveBeenCalledWith('loadout-1');
    expect(await screen.findByText('Combo removido')).toBeInTheDocument();
  });
});
