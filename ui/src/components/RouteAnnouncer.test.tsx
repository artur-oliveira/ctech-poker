import {render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {RouteAnnouncer} from './RouteAnnouncer';

const mocks = vi.hoisted(() => ({pathname: '/lobby'}));
vi.mock('next/navigation', () => ({usePathname: () => mocks.pathname}));

function mountPage(html: string) {
  const page = document.createElement('div');
  page.innerHTML = html;
  document.body.appendChild(page);
  return page;
}

beforeEach(() => {
  mocks.pathname = '/lobby';
  document.title = 'Lobby · CTech Poker';
});

afterEach(() => {
  document.querySelectorAll('main').forEach(m => m.parentElement?.remove());
});

describe('RouteAnnouncer', () => {
  test('stays silent and leaves focus alone on first render', () => {
    mountPage('<main><h1>Lobby</h1></main>');
    render(<RouteAnnouncer/>);
    expect(screen.getByRole('status')).toHaveTextContent('');
    expect(screen.getByRole('heading')).not.toHaveFocus();
  });

  test('moves focus to the new page heading and announces its title after a route change', () => {
    const page = mountPage('<main><h1>Lobby</h1></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    document.title = 'Mesa de poker · CTech Poker';
    page.innerHTML = '<main><h1>Mesa de poker</h1></main>';
    mocks.pathname = '/table/room-1';
    rerender(<RouteAnnouncer/>);
    expect(screen.getByRole('heading')).toHaveFocus();
    expect(screen.getByRole('heading')).toHaveAttribute('tabindex', '-1');
    expect(screen.getByRole('status')).toHaveTextContent('Mesa de poker · CTech Poker');
  });

  test('falls back to focusing main when the page has no heading', () => {
    mountPage('<main></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    mocks.pathname = '/leaderboard';
    rerender(<RouteAnnouncer/>);
    expect(document.querySelector('main')).toHaveFocus();
  });
});
