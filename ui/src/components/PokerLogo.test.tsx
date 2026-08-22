import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {PokerLogo} from './PokerLogo';

describe('PokerLogo', () => {
  test('renders the canonical product mark at the requested size', () => {
    const {container} = render(<PokerLogo size={32} className="product-mark" priority/>);
    const logo = container.querySelector('img');

    expect(logo).toHaveAttribute('src', '/svgs/logo.svg');
    // Requested at 3x the display size so Safari's SVG-in-<img> raster cache
    // stays crisp on Retina screens; CSS (not these attrs) sets the actual box.
    expect(logo).toHaveAttribute('width', '96');
    expect(logo).toHaveAttribute('height', '96');
    expect(logo).toHaveClass('product-mark');
    expect(logo).toHaveAttribute('aria-hidden', 'true');
  });
});
