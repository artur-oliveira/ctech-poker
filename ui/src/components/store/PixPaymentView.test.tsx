import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
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
});
