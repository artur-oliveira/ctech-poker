import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {PromoCodeForm} from './PromoCodeForm';
import {ApiError} from '@/lib/api/client';
import type {SandboxPurchase} from '@/lib/api/wallet';

const {createPurchase} = vi.hoisted(() => ({createPurchase: vi.fn()}));
vi.mock('@/lib/api/wallet', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/wallet')>(),
  createPurchase,
}));

const purchase: SandboxPurchase = {
  purchase_id: 'sbxp-1', sku: 'pack_promo', status: 'pending', promo_code: 'BEMVINDO10',
};

describe('PromoCodeForm', () => {
  beforeEach(() => {
    createPurchase.mockReset();
  });

  test('the apply button stays disabled until a code is typed', () => {
    render(<PromoCodeForm onRedeemedAction={vi.fn()}/>);
    expect(screen.getByRole('button', {name: /Aplicar código/})).toBeDisabled();
  });

  test('redeems a code and hands the resulting purchase to the caller', async () => {
    createPurchase.mockResolvedValue(purchase);
    const onRedeemedAction = vi.fn();
    render(<PromoCodeForm onRedeemedAction={onRedeemedAction}/>);

    await userEvent.type(screen.getByRole('textbox', {name: /Código promocional/}), 'bemvindo10');
    await userEvent.click(screen.getByRole('button', {name: /Aplicar código/}));

    expect(createPurchase).toHaveBeenCalledWith('', 'BEMVINDO10');
    expect(onRedeemedAction).toHaveBeenCalledWith(purchase);
    expect(await screen.findByRole('status')).toHaveTextContent('Código aplicado');
    // The input clears on success so the field is ready for a different code.
    expect(screen.getByRole('textbox', {name: /Código promocional/})).toHaveValue('');
  });

  test('shows a friendly message for an already-redeemed code, without opening a purchase', async () => {
    createPurchase.mockRejectedValue(new ApiError('bad request', 400, {detail: 'promocode: already redeemed by this player'}));
    const onRedeemedAction = vi.fn();
    render(<PromoCodeForm onRedeemedAction={onRedeemedAction}/>);

    await userEvent.type(screen.getByRole('textbox', {name: /Código promocional/}), 'USADO');
    await userEvent.click(screen.getByRole('button', {name: /Aplicar código/}));

    expect(await screen.findByRole('alert')).toHaveTextContent('Este código já foi usado na sua conta.');
    expect(onRedeemedAction).not.toHaveBeenCalled();
  });

  test('shows a friendly message for an expired code', async () => {
    createPurchase.mockRejectedValue(new ApiError('bad request', 400, {detail: 'promocode: expired'}));
    render(<PromoCodeForm onRedeemedAction={vi.fn()}/>);

    await userEvent.type(screen.getByRole('textbox', {name: /Código promocional/}), 'VENCIDO');
    await userEvent.click(screen.getByRole('button', {name: /Aplicar código/}));

    expect(await screen.findByRole('alert')).toHaveTextContent('Este código promocional expirou.');
  });
});
