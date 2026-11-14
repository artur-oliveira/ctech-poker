import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {
  AccessoryStoreSection, cosmeticActivityRows, DeckStoreSection, FeltStoreSection, FrameStoreSection, HatStoreSection
} from './CosmeticStoreSection';
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
    expect(screen.getAllByText('A liberar').length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', {name: 'Liberar'})[0]);
    expect(actions.onBuyAction).toHaveBeenCalledWith(expect.objectContaining({id: 'golden'}), expect.any(HTMLButtonElement));
  });

  test('an owned premium deck shows an owned state and can be refunded, no buy button', async () => {
    renderSection([purchase()], {catalog: ownedGoldenCatalog});
    const items = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getAllByRole('listitem');
    const owned = items.find(item => within(item).queryByText('Na coleção'));
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
    const owned = items.find(item => within(item).queryByText('Na coleção'));
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

describe('avatar item sections', () => {
  const actions = {onRetryAction: vi.fn(), onBuyAction: vi.fn(), onRefundAction: vi.fn(), onResumeAction: vi.fn()};
  beforeEach(() => Object.values(actions).forEach(action => action.mockReset()));

  const hatCatalog: CosmeticCatalogEntry[] = [
    {kind: 'hat', id: 'top-hat', premium: true, owned: false, source: 'purchase', available: true,
      price_cents: 300, price_fichas: 300_000},
    {kind: 'hat', id: 'crown', premium: true, owned: false, source: 'achievement', available: true},
    {kind: 'hat', id: 'cowboy', premium: true, owned: false, source: 'purchase', available: false,
      price_cents: 400, price_fichas: 400_000},
    {kind: 'hat', id: 'beanie', premium: true, owned: true, source: 'purchase', available: true,
      price_cents: 200, price_fichas: 200_000},
    {kind: 'hat', id: 'not-a-real-hat', premium: true, owned: false, source: 'purchase', available: true,
      price_cents: 100, price_fichas: 100_000},
  ];
  const renderHats = (purchases: CosmeticPurchase[] = []) =>
    render(<HatStoreSection catalog={hatCatalog} purchases={purchases} isLoading={false} isError={false} {...actions}/>);
  const item = (name: string) => within(screen.getByRole('list', {name: 'Catálogo de chapéus'}))
    .getAllByRole('listitem').find(row => within(row).queryByText(name))!;

  test('hides ids this build has no art or label for', () => {
    renderHats();
    expect(within(screen.getByRole('list', {name: 'Catálogo de chapéus'})).getAllByRole('listitem')).toHaveLength(4);
    expect(screen.queryByText('not-a-real-hat')).not.toBeInTheDocument();
  });

  test('a purchasable unowned hat shows its price and can be bought', async () => {
    renderHats();
    const row = item('Cartola');
    expect(within(row).getByText(/R\$\s?3,00/)).toBeInTheDocument();
    expect(within(row).getByText(/300\.000 fichas/)).toBeInTheDocument();
    await userEvent.click(within(row).getByRole('button', {name: 'Liberar'}));
    expect(actions.onBuyAction).toHaveBeenCalledWith(expect.objectContaining({kind: 'hat', id: 'top-hat'}),
      expect.any(HTMLButtonElement));
  });

  test('an earnable hat is locked behind an achievement, with no price and no buy button', () => {
    renderHats();
    const row = item('Coroa');
    expect(within(row).getByText('Conquista')).toBeInTheDocument();
    expect(row.querySelector('.cosmetic-store-lock svg')).not.toBeNull();
    expect(within(row).queryByText(/fichas/)).not.toBeInTheDocument();
    expect(within(row).queryByRole('button')).not.toBeInTheDocument();
  });

  test('an unowned hat whose sale window closed is unavailable, with no buy button', () => {
    renderHats();
    const row = item('Chapéu de Cowboy');
    expect(within(row).getByText('Indisponível')).toBeInTheDocument();
    expect(within(row).queryByRole('button')).not.toBeInTheDocument();
  });

  test('a pending Pix on a closed-window hat still resumes', async () => {
    const pending = {...purchase({kind: 'hat', item_id: 'cowboy', status: 'pending'})};
    renderHats([pending]);
    await userEvent.click(within(item('Chapéu de Cowboy')).getByRole('button', {name: /Acompanhar/}));
    expect(actions.onResumeAction).toHaveBeenCalledWith(expect.objectContaining({id: 'cowboy'}), pending,
      expect.any(HTMLButtonElement));
  });

  test('an owned earned hat shows no price', () => {
    render(<HatStoreSection catalog={[{kind: 'hat', id: 'crown', premium: true, owned: true, source: 'achievement',
      available: true}]} purchases={[]} isLoading={false} isError={false} {...actions}/>);
    expect(within(item('Coroa')).getByText('Na coleção')).toBeInTheDocument();
    expect(within(item('Coroa')).queryByText(/R\$|fichas/)).not.toBeInTheDocument();
  });

  test('a premium entry the server sent without a price is never sellable', () => {
    render(<DeckStoreSection catalog={[{kind: 'deck', id: 'golden', premium: true, owned: false}]} purchases={[]}
      isLoading={false} isError={false} {...actions}/>);
    const row = within(screen.getByRole('list', {name: 'Catálogo de baralhos'})).getByRole('listitem');
    expect(within(row).getByText('Indisponível')).toBeInTheDocument();
    expect(within(row).queryByText(/R\$|fichas/)).not.toBeInTheDocument();
    expect(within(row).queryByRole('button')).not.toBeInTheDocument();
  });

  test('an owned hat links to the profile to equip it', () => {
    renderHats();
    const row = item('Gorro');
    expect(within(row).getByText('Na coleção')).toBeInTheDocument();
    expect(within(row).getByRole('link', {name: /Equipar no perfil/})).toHaveAttribute('href', '/player-profile');
    expect(within(row).queryByRole('button', {name: 'Liberar'})).not.toBeInTheDocument();
  });

  test('a season item is locked as Temporada, not Conquista, with no price', () => {
    render(<FrameStoreSection catalog={[{kind: 'frame', id: 'champion-gold', premium: true, source: 'season',
      available: true}]} purchases={[]} isLoading={false} isError={false} {...actions}/>);
    const list = screen.getByRole('list', {name: 'Catálogo de molduras'});
    expect(within(list).getByText('Temporada')).toBeInTheDocument();
    expect(within(list).queryByText('Conquista')).not.toBeInTheDocument();
    expect(within(list).queryByText(/fichas/)).not.toBeInTheDocument();
    expect(within(list).queryByRole('button')).not.toBeInTheDocument();
  });

  test('frames and accessories render their own catalogs', () => {
    render(<>
      <FrameStoreSection catalog={[{kind: 'frame', id: 'felt-ring', premium: true, source: 'purchase', available: true,
        price_cents: 600, price_fichas: 600_000}]} purchases={[]} isLoading={false} isError={false} {...actions}/>
      <AccessoryStoreSection catalog={[{kind: 'accessory', id: 'bandit-mask', premium: true, source: 'achievement',
        available: true}]} purchases={[]} isLoading={false} isError={false} {...actions}/>
    </>);
    expect(within(screen.getByRole('list', {name: 'Catálogo de molduras'})).getByText('Borda de Feltro')).toBeInTheDocument();
    expect(within(screen.getByRole('list', {name: 'Catálogo de acessórios'})).getByText('Conquista')).toBeInTheDocument();
  });
});

describe('cosmeticActivityRows', () => {
  test('labels avatar item purchases by kind', () => {
    const rows = cosmeticActivityRows([
      purchase({purchase_id: 'a', kind: 'frame', item_id: 'walnut-rail'}),
      purchase({purchase_id: 'b', kind: 'hat', item_id: 'top-hat', method: 'fichas'}),
      purchase({purchase_id: 'c', kind: 'accessory', item_id: 'monocle'}),
    ]);
    expect(rows.map(row => [row.label, row.detail.split(' · ').slice(0, 2).join(' · ')])).toEqual([
      ['Borda de Nogueira', 'Borda · Pix'], ['Cartola', 'Chapéu · Fichas'], ['Monóculo', 'Acessório · Pix'],
    ]);
    expect(rows.every(row => row.media)).toBe(true);
  });
});
