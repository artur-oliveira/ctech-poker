import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {PokerLogo} from './PokerLogo';

describe('PokerLogo', () => {
  test('renders the canonical product mark at the requested size', () => {
    const {container} = render(<PokerLogo size={32} className="product-mark" priority/>);
    const logo = container.querySelector('img');

    expect(logo).toHaveAttribute('src', '/svgs/logo.svg');
    expect(logo).toHaveAttribute('width', '32');
    expect(logo).toHaveAttribute('height', '32');
    expect(logo).toHaveClass('product-mark');
    expect(logo).toHaveAttribute('aria-hidden', 'true');
  });
});
