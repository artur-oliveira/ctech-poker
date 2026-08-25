import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {DeckStoreSection, FeltStoreSection} from './CosmeticStoreSection';
import type {CosmeticCatalogEntry, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';

vi.mock('next/image', () => ({default: ({alt}: {alt: string}) => <div role="img" aria-label={alt}/>}));

const deckCatalog: CosmeticCatalogEntry[] = [
  {kind: 'deck', id: 'four-color', premium: false, owned: true},
  {kind: 'deck', id: 'golden', premium: true, owned: false, price_cents: 2990, price_fichas: 500_000},
  {kind: 'deck', id: 'pink', premium: true, owned: false, price_cents: 2990, price_fichas: 500_000},
  {kind: 'deck', id: 'not-a-real-deck', premium: true, owned: false, price_cents: 100, price_fichas: 100},
];

// Ownership is a catalog fact now (the server reads it from entitlements), so
// "owning golden" is a different catalog, not a different purchase list.
const ownedGoldenCatalog = deckCatalog.map(entry => entry.id === 'golden' ? {...entry, owned: true} : entry);

const feltCatalog: CosmeticCatalogEntry[] = [
  {kind: 'felt', id: 'classic', premium: false, owned: true},
  {kind: 'felt', id: 'midnight', premium: true, owned: false, price_cents: 990, price_fichas: 200_000},
];

function purchase(overrides: Partial<CosmeticPurchase> = {}): CosmeticPurchase {
  return {purchase_id: 'p1', kind: 'deck', item_id: 'golden', method: 'pix', status: 'confirmed', ...overrides};
}

describe('DeckStoreSection', () => {
  const actions = {onRetryAction: vi.fn(), onBuyAction: vi.fn(), onRefundAction: vi.fn(), onResumeAction: vi.fn()};
  beforeEach(() => Object.values(actions).forEach(action => action.mockReset()));

  const renderSection = (purchases: CosmeticPurchase[] = [], overrides = {}) =>
    render(<DeckStoreSection catalog={deckCatalog} purchases={purchases} isLoading={false} isError={false}
      {...actions} {...overrides}/>);

  test('shows a skeleton while loading', () => {
    renderSection([], {isLoading: true});
    expect(screen.getByText('Carregando baralhos…')).toBeInTheDocument();
  });

  test('offers a retry on error', async () => {
    renderSection([], {isError: true});
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    expect(actions.onRetryAction).toHaveBeenCalledOnce();
  });

  test('falls back to an empty state when the catalog is empty', () => {
    renderSection([], {catalog: []});
    expect(screen.getByText('Nenhum baralho disponível no momento.')).toBeInTheDocument();
  });

  test('renders every recognized catalog entry, free and premium, skipping unknown ids', () => {
    renderSection();
    const items = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getAllByRole('listitem');
    expect(items).toHaveLength(3);
    expect(screen.queryByText('not-a-real-deck')).not.toBeInTheDocument();
  });

  test('a free deck shows no price and no buy/refund action', () => {
    renderSection();
    const items = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getAllByRole('listitem');
    const free = items.find(item => within(item).queryByText('Grátis'));
    expect(free).toBeDefined();
    expect(within(free!).queryByRole('button')).not.toBeInTheDocument();
  });

  test('an unowned premium deck can be bought', async () => {
    renderSection();
    expect(screen.getAllByText('Não liberada').length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', {name: 'Liberar'})[0]);
    expect(actions.onBuyAction).toHaveBeenCalledWith(expect.objectContaining({id: 'golden'}), expect.any(HTMLButtonElement));
  });

  test('an owned premium deck shows an owned state and can be refunded, no buy button', async () => {
    renderSection([purchase()], {catalog: ownedGoldenCatalog});
    const items = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getAllByRole('listitem');
    const owned = items.find(item => within(item).queryByText('Sua'));
    expect(owned).toBeDefined();
    expect(within(owned!).queryByRole('button', {name: 'Liberar'})).not.toBeInTheDocument();
    await userEvent.click(within(owned!).getByRole('button', {name: /Estornar/}));
    expect(actions.onRefundAction).toHaveBeenCalledWith(expect.objectContaining({purchase_id: 'p1'}), expect.any(HTMLButtonElement));
  });

  test('an owned deck whose receipt is not on this page of history offers no refund button', () => {
    // Ownership comes from the catalog and history is paginated, so the two can
    // legitimately disagree; the owned state must still render.
    renderSection([], {catalog: ownedGoldenCatalog});
    const items = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getAllByRole('listitem');
    const owned = items.find(item => within(item).queryByText('Sua'));
    expect(owned).toBeDefined();
    expect(within(owned!).queryByRole('button')).not.toBeInTheDocument();
  });

  test('a pending purchase resumes instead of buying again', async () => {
    const pending = purchase({status: 'pending'});
    renderSection([pending]);
    expect(screen.getByText('Aguardando Pix')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Acompanhar/}));
    expect(actions.onResumeAction).toHaveBeenCalledWith(expect.objectContaining({id: 'golden'}), pending, expect.any(HTMLButtonElement));
  });

  test('a refunding purchase blocks further action', () => {
    renderSection([purchase({status: 'refunding'})]);
    expect(screen.getByText('Estornando')).toBeInTheDocument();
    expect(screen.getByText('Aguarde')).toBeInTheDocument();
  });
});

describe('FeltStoreSection', () => {
  const actions = {onRetryAction: vi.fn(), onBuyAction: vi.fn(), onRefundAction: vi.fn(), onResumeAction: vi.fn()};
  beforeEach(() => Object.values(actions).forEach(action => action.mockReset()));

  test('renders felt swatches for every catalog entry', () => {
    render(<FeltStoreSection catalog={feltCatalog} purchases={[]} isLoading={false} isError={false} {...actions}/>);
    const items = within(screen.getByRole('list', {name: 'Catálogo de feltros'})).getAllByRole('listitem');
    expect(items).toHaveLength(2);
    expect(screen.getByText('Clássico')).toBeInTheDocument();
    expect(screen.getByText('Meia-noite')).toBeInTheDocument();
  });

  test('buys an unowned premium felt', async () => {
    render(<FeltStoreSection catalog={feltCatalog} purchases={[]} isLoading={false} isError={false} {...actions}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Liberar'}));
    expect(actions.onBuyAction).toHaveBeenCalledWith(expect.objectContaining({id: 'midnight'}), expect.any(HTMLButtonElement));
  });
});
