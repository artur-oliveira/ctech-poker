import {createContext, type ReactNode, useContext} from 'react';
import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';

import {TablePreferencesDialog} from './TablePreferencesDialog';
import type {CosmeticCatalogEntry} from '@/lib/api/cosmeticPurchases';
import type {PlayerProfile} from '@/lib/api/player';

const {useTablePreferences, update, getMe, updateMe, listCosmeticCatalog, listCosmeticPurchases} = vi.hoisted(() => ({
  useTablePreferences: vi.fn(),
  update: vi.fn(),
  getMe: vi.fn(),
  updateMe: vi.fn(),
  listCosmeticCatalog: vi.fn(),
  listCosmeticPurchases: vi.fn(),
}));

vi.mock('@/lib/tablePreferences', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/tablePreferences')>(),
  useTablePreferences,
}));
vi.mock('@/lib/api/player', () => ({getMe, updateMe}));
vi.mock('@/lib/api/cosmeticPurchases', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/cosmeticPurchases')>(),
  listCosmeticCatalog, listCosmeticPurchases,
}));

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({children}: React.PropsWithChildren) => <div>{children}</div>,
  DialogTrigger: ({children, render}: React.PropsWithChildren<{
    render?: React.ReactElement<{ ['aria-label']?: string }>;
  }>) => <button type="button" aria-label={render?.props['aria-label']}>{children}</button>,
  DialogContent: ({children}: React.PropsWithChildren) => <section>{children}</section>,
  DialogHeader: ({children}: React.PropsWithChildren) => <header>{children}</header>,
  DialogTitle: ({children}: React.PropsWithChildren) => <h2>{children}</h2>,
  DialogDescription: ({children}: React.PropsWithChildren) => <p>{children}</p>,
}));

vi.mock('@/components/ui/select', () => {
  const ChangeContext = createContext<(value: string | null) => void>(() => undefined);
  return {
    Select: ({children, onValueChange}: React.PropsWithChildren<{ onValueChange: (value: never) => void }>) =>
      <ChangeContext.Provider value={onValueChange as unknown as (value: string | null) => void}>
        {children}
      </ChangeContext.Provider>,
    SelectTrigger: ({children, ...props}: React.PropsWithChildren) => <div {...props}>{children}</div>,
    SelectValue: ({children}: { children: (value: never) => React.ReactNode }) =>
      <>{children('classic' as never)}</>,
    SelectContent: ({children}: React.PropsWithChildren) => <div>{children}</div>,
    SelectItem: ({children, value}: React.PropsWithChildren<{ value: string }>) => {
      const change = useContext(ChangeContext);
      return <button type="button" onClick={() => change(value)}>{children}</button>;
    },
  };
});

vi.mock('@/components/ui/switch', () => ({
  Switch: ({checked, onCheckedChange, ...props}: {
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
    [key: string]: unknown;
  }) => <button type="button" role="switch" aria-checked={checked}
                onClick={() => onCheckedChange(!checked)} {...props}/>,
}));

const player = (overrides: Partial<PlayerProfile> = {}): PlayerProfile => ({
  user_id: 'p1', wallet_mode: 'sandbox', poker_terms_accepted: true, showcase_public: true,
  playstyle_public: true, table_public: false, ...overrides,
});

const catalog: CosmeticCatalogEntry[] = [
  {kind: 'felt', id: 'classic', premium: false, owned: true},
  {kind: 'felt', id: 'midnight', premium: true, owned: false, price_fichas: 200_000},
  {kind: 'felt', id: 'burgundy', premium: true, owned: false, price_fichas: 200_000},
  {kind: 'felt', id: 'ocean', premium: true, owned: false, price_fichas: 200_000},
];

// Ownership is a catalog fact now (the server reads it from entitlements).
const ownedMidnightCatalog = catalog.map(entry => entry.id === 'midnight' ? {...entry, owned: true} : entry);


function wrapper({children}: {children: ReactNode}) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function renderDialog(props: Partial<React.ComponentProps<typeof TablePreferencesDialog>> = {}) {
  // The mocked Dialog renders its content unconditionally, so `open` here only
  // drives the catalog's `enabled` latch — the closed case is asserted below.
  return render(<TablePreferencesDialog open {...props}/>, {wrapper});
}

describe('TablePreferencesDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useTablePreferences.mockReturnValue({
      preferences: {
        soundEffects: false, dealerVoice: false, voiceCommands: true, realityCheckMinutes: 60, equityTrainer: false,
        keyboardShortcuts: true
      },
      update,
    });
    getMe.mockResolvedValue(player());
    updateMe.mockResolvedValue(player({table_theme: 'midnight'}));
    listCosmeticCatalog.mockResolvedValue(catalog);
    listCosmeticPurchases.mockResolvedValue([]);
  });

  test('renders the current preferences and all supported choices', async () => {
    renderDialog();

    expect(screen.getByRole('button', {name: 'Preferências da mesa'})).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: 'Preferências da mesa'})).toBeInTheDocument();
    expect(screen.getByText('Personalize a experiência e escolha como prefere jogar nesta mesa.')).toBeInTheDocument();
    expect(screen.getAllByText('Clássico').length).toBeGreaterThan(0);
    expect(await screen.findByRole('button', {name: /Meia-noite/})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Bordô/})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Oceano/})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Desativado'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'A cada 2 horas'})).toBeInTheDocument();
    expect(screen.getAllByRole('switch')).toHaveLength(5);
    expect(screen.getByRole('switch', {name: 'Sons da mesa'})).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByRole('switch', {name: 'Dealer auditivo'})).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByRole('switch', {name: 'Comandos por voz'})).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByRole('switch', {name: 'Treinador'})).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByRole('switch', {name: 'Atalhos de teclado'})).toHaveAttribute('aria-checked', 'true');
  });

  test('hides the shortcut legend once shortcuts are turned off', async () => {
    renderDialog();
    expect(screen.getByText('Fold')).toBeInTheDocument();
    useTablePreferences.mockReturnValue({
      preferences: {
        soundEffects: false, dealerVoice: false, voiceCommands: true, realityCheckMinutes: 60, equityTrainer: false,
        keyboardShortcuts: false
      },
      update,
    });
    renderDialog();
    const toggle = screen.getAllByRole('switch', {name: 'Atalhos de teclado'})[1];
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(screen.getAllByText('Fold')).toHaveLength(1);
  });

  test('does not fetch the felt catalog while the dialog is closed', async () => {
    renderDialog({open: false});

    expect(await screen.findByRole('heading', {name: 'Preferências da mesa'})).toBeInTheDocument();
    expect(listCosmeticCatalog).not.toHaveBeenCalled();
  });

  test('waits for the catalog before calling a premium felt locked', async () => {
    listCosmeticCatalog.mockReturnValue(new Promise(() => undefined));
    renderDialog();

    const midnight = await screen.findByRole('button', {name: 'Meia-noite'});
    expect(within(midnight).queryByLabelText(/Premium bloqueado/)).not.toBeInTheDocument();
  });

  test('shows the run-it-twice choice only when the room allows it', async () => {
    const onChange = vi.fn(() => true);
    const hidden = renderDialog();
    expect(screen.queryByText('Rodar duas vezes')).not.toBeInTheDocument();
    hidden.unmount();

    renderDialog({runItTwiceAvailable: true, runItTwice: false, onRunItTwiceChange: onChange});
    expect(screen.getByText(/todos os jogadores envolvidos também ativaram/)).toBeInTheDocument();
    const toggle = screen.getByRole('switch', {name: 'Rodar duas vezes'});
    await userEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(true);
  });

  test('updates voice settings and session reminder through the local preferences store', async () => {
    renderDialog();

    await userEvent.click(screen.getByRole('switch', {name: 'Sons da mesa'}));
    await userEvent.click(screen.getByRole('switch', {name: 'Dealer auditivo'}));
    await userEvent.click(screen.getByRole('switch', {name: 'Comandos por voz'}));
    await userEvent.click(screen.getByRole('button', {name: 'A cada 30 minutos'}));

    await userEvent.click(screen.getByRole('switch', {name: 'Treinador'}));

    expect(update).toHaveBeenCalledWith({soundEffects: true});
    expect(update).toHaveBeenCalledWith({dealerVoice: true});
    expect(update).toHaveBeenCalledWith({voiceCommands: false});
    expect(update).toHaveBeenCalledWith({equityTrainer: true});
    expect(update).toHaveBeenCalledWith({realityCheckMinutes: 30});
    expect(update).toHaveBeenCalledTimes(5);
  });

  test('selecting the free classic felt persists via updateMe, not localStorage', async () => {
    getMe.mockResolvedValue(player({table_theme: 'midnight'}));
    renderDialog();
    await screen.findByRole('button', {name: /Meia-noite/});

    await userEvent.click(screen.getByRole('button', {name: 'Clássico'}));
    expect(updateMe).toHaveBeenCalledWith({table_theme: 'classic'}, expect.anything());
    expect(update).not.toHaveBeenCalledWith(expect.objectContaining({theme: expect.anything()}));
  });

  test('selecting an unowned premium felt does not mutate and instead reports the lock', async () => {
    const onLockedFeltAction = vi.fn();
    renderDialog({onLockedFeltAction});
    const midnight = await screen.findByRole('button', {name: /Meia-noite/});
    expect(within(midnight).getByLabelText(/Premium bloqueado/)).toBeInTheDocument();

    await userEvent.click(midnight);
    expect(onLockedFeltAction).toHaveBeenCalledWith('midnight');
    expect(updateMe).not.toHaveBeenCalled();
  });

  test('selecting an owned premium felt mutates normally with no lock icon', async () => {
    listCosmeticCatalog.mockResolvedValue(ownedMidnightCatalog);
    const onLockedFeltAction = vi.fn();
    renderDialog({onLockedFeltAction});
    const midnight = await screen.findByRole('button', {name: 'Meia-noite'});
    expect(within(midnight).queryByLabelText(/Premium bloqueado/)).not.toBeInTheDocument();

    await userEvent.click(midnight);
    expect(updateMe).toHaveBeenCalledWith({table_theme: 'midnight'}, expect.anything());
    expect(onLockedFeltAction).not.toHaveBeenCalled();
  });
});
