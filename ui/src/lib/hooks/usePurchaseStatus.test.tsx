import type {ReactNode} from 'react';
import {renderHook} from '@testing-library/react';
import {focusManager, QueryClientProvider, type QueryClient} from '@tanstack/react-query';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {createQueryClient} from '@/lib/providers/createQueryClient';
import {
  nextPurchasePollMs, purchaseActive, purchasePollCount, PURCHASE_POLL_BUDGET, PURCHASE_POLL_MAX_MS,
  PURCHASE_POLL_MS, resetPurchasePollCount, usePurchaseStatus
} from './usePurchaseStatus';

type Purchase = { purchase_id: string; status: string; expires_at?: string };

const pending = (overrides: Partial<Purchase> = {}): Purchase => ({
  purchase_id: 'p-1', status: 'pending',
  expires_at: new Date(Date.now() + 15 * 60_000).toISOString(), ...overrides,
});

let client: QueryClient;
let fetchStatus: ReturnType<typeof vi.fn<() => Promise<Purchase>>>;

function wrapper({children}: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function render(purchase: Purchase | undefined) {
  return renderHook(() => usePurchaseStatus<Purchase>({
    queryKey: ['wallet', 'test-purchase', purchase?.purchase_id ?? ''],
    queryFn: () => fetchStatus(),
    purchase,
  }), {wrapper});
}

describe('nextPurchasePollMs', () => {
  test('starts at the base gap and doubles up to the ceiling', () => {
    expect(nextPurchasePollMs(0)).toBe(PURCHASE_POLL_MS);
    expect(nextPurchasePollMs(4)).toBe(PURCHASE_POLL_MS);
    expect(nextPurchasePollMs(5)).toBe(PURCHASE_POLL_MS * 2);
    expect(nextPurchasePollMs(10)).toBe(PURCHASE_POLL_MS * 4);
    expect(nextPurchasePollMs(30)).toBe(PURCHASE_POLL_MAX_MS);
  });

  test('gives up at the budget and when the Pix window has closed', () => {
    expect(nextPurchasePollMs(PURCHASE_POLL_BUDGET)).toBe(false);
    expect(nextPurchasePollMs(0, -1)).toBe(false);
    expect(nextPurchasePollMs(0, 60_000)).toBe(PURCHASE_POLL_MS);
  });
});

describe('purchaseActive', () => {
  test('only pending and processing are still waiting on the provider', () => {
    expect(purchaseActive('pending')).toBe(true);
    expect(purchaseActive('processing')).toBe(true);
    for (const status of ['confirmed', 'expired', 'failed', 'refunding', 'refunded', undefined]) {
      expect(purchaseActive(status)).toBe(false);
    }
  });
});

describe('usePurchaseStatus', () => {
  beforeEach(() => {
    resetPurchasePollCount();
    fetchStatus = vi.fn().mockResolvedValue(pending());
    // The real app defaults, so `refetchOnWindowFocus` and the rest are the
    // ones actually shipped.
    client = createQueryClient();
    focusManager.setFocused(true);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    focusManager.setFocused(undefined);
  });

  test('does not read anything for a purchase that is already resolved', async () => {
    render(pending({status: 'confirmed'}));
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS * 4);
    expect(purchasePollCount()).toBe(0);
  });

  test('opening on a known row spends no read of its own', async () => {
    const {result} = render(pending());
    expect(result.current.data?.status).toBe('pending');
    expect(purchasePollCount()).toBe(0);
  });

  test('falls back to a poll on the base gap while the purchase is pending', async () => {
    render(pending());
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    expect(purchasePollCount()).toBe(1);
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    expect(purchasePollCount()).toBe(2);
  });

  test('a hidden tab spends nothing, and coming back spends exactly one read', async () => {
    render(pending());
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    expect(purchasePollCount()).toBe(1);

    focusManager.setFocused(false);
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS * 10);
    expect(purchasePollCount()).toBe(1);

    focusManager.setFocused(true);
    await vi.waitFor(() => expect(purchasePollCount()).toBe(2));
    // Still one: the catch-up read is the query's own focus refetch, not a
    // backlog of every interval the hidden tab skipped.
    expect(purchasePollCount()).toBe(2);
  });

  test('stops the moment the fetched status is no longer waiting', async () => {
    fetchStatus.mockResolvedValue(pending({status: 'confirmed'}));
    const {result} = render(pending());
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    await vi.waitFor(() => expect(result.current.data?.status).toBe('confirmed'));
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS * 10);
    expect(purchasePollCount()).toBe(1);
  });

  test('stops when the Pix window has closed', async () => {
    fetchStatus.mockResolvedValue(pending({expires_at: new Date(Date.now() - 1000).toISOString()}));
    render(pending());
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    expect(purchasePollCount()).toBe(1);
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS * 10);
    expect(purchasePollCount()).toBe(1);
  });

  test('a failed poll surfaces the error and keeps the fallback going', async () => {
    fetchStatus.mockRejectedValueOnce(new Error('offline')).mockResolvedValue(pending());
    const {result} = render(pending());
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    await vi.waitFor(() => expect(result.current.isError).toBe(true));
    await vi.advanceTimersByTimeAsync(PURCHASE_POLL_MS);
    await vi.waitFor(() => expect(result.current.isError).toBe(false));
    expect(purchasePollCount()).toBe(2);
  });
});
