import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {PixPaymentView} from './PixPaymentView';
import type {SandboxPurchase} from '@/lib/api/wallet';

const purchase: SandboxPurchase = {
  purchase_id: 'sbxp-1',
  sku: 'pack_100',
  status: 'pending',
  qr_code_base64: 'aGVsbG8=',
  pix_copia_e_cola: '00020126...',
  expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
};

describe('PixPaymentView', () => {
  test('renders the QR code and pix copia-e-cola field for a pending purchase', () => {
    render(<PixPaymentView purchase={purchase}/>);
    expect(screen.getByLabelText(/pix copia e cola/i)).toHaveValue('00020126...');
    expect(screen.getByAltText(/qr code pix/i)).toBeInTheDocument();
  });

  test('copies the code and confirms it to assistive tech', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText}});
    render(<PixPaymentView purchase={purchase}/>);

    await userEvent.click(screen.getByRole('button', {name: 'Copiar código Pix'}));
    expect(writeText).toHaveBeenCalledWith('00020126...');
    expect(await screen.findByRole('button', {name: 'Código Pix copiado'})).toBeInTheDocument();
    expect(screen.getByText('Código Pix copiado.')).toBeInTheDocument();
  });

  test('falls back to manual copying on a browser without clipboard access', async () => {
    Object.defineProperty(navigator, 'clipboard', {configurable: true, value: undefined});
    render(<PixPaymentView purchase={purchase}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Copiar código Pix'}));
    expect(await screen.findByRole('alert')).toHaveTextContent('Selecione o código acima e copie manualmente.');
  });

  test('selects the whole code when the field is clicked', async () => {
    render(<PixPaymentView purchase={purchase}/>);
    const field = screen.getByLabelText('Pix copia e cola') as HTMLInputElement;
    const select = vi.spyOn(field, 'select');
    await userEvent.click(field);
    expect(select).toHaveBeenCalled();
  });

  test('disables copying and shows the countdown as expired once the code lapses', () => {
    render(<PixPaymentView purchase={{...purchase, expires_at: new Date(Date.now() - 1000).toISOString()}}/>);
    expect(screen.getByRole('button', {name: 'Código Pix expirado'})).toBeDisabled();
    expect(screen.getByText('Código expirado')).toBeInTheDocument();
  });

  test('counts down while the code is still valid', () => {
    render(<PixPaymentView purchase={purchase}/>);
    expect(screen.getByText(/Expira em \d+:\d\d/)).toBeInTheDocument();
  });

  test('renders an SVG QR code and a custom payment note without a deadline', () => {
    render(<PixPaymentView purchase={{qr_code_base64: 'PHN2Zy'}}
      paymentNote="Apenas cosmético."/>);
    expect(screen.getByAltText(/qr code pix/i)).toBeInTheDocument();
    expect(screen.getByText('Apenas cosmético.')).toBeInTheDocument();
    expect(screen.queryByText(/Expira em/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Copiar código Pix'})).toBeDisabled();
  });
});
