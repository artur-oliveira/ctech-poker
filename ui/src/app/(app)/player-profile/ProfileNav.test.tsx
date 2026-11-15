import {render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ProfileNav} from './ProfileNav';

const flags = vi.hoisted(() => ({enabled: false}));
vi.mock('@/lib/capabilities', () => ({get REAL_MONEY_UI_ENABLED() { return flags.enabled; }}));

describe('ProfileNav', () => {
  beforeEach(() => { flags.enabled = false; });

  test('links every section by its title id, without the real-money one while it is off', () => {
    render(<ProfileNav/>);
    const links = screen.getAllByRole('link');
    expect(links.map(link => link.textContent)).toEqual(
      ['Identidade', 'Avatar', 'Vitrine', 'Mesa', 'Chat', 'Saldos', 'Alertas']);
    expect(screen.getByRole('link', {name: 'Saldos'})).toHaveAttribute('href', '#player-profile-balances-title');
  });

  test('adds Jogo responsável where real money is on', () => {
    flags.enabled = true;
    render(<ProfileNav/>);
    expect(screen.getByRole('link', {name: 'Jogo responsável'}))
      .toHaveAttribute('href', '#player-profile-responsible-title');
  });
});
