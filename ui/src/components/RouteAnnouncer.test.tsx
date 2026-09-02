import {StrictMode} from 'react';
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

  // The regression this guards: with a "have I mounted" boolean, StrictMode's
  // second mount-effect pass fell through and set tabindex on main/h1 while a
  // Suspense boundary below was still hydrating, which React reports as a
  // hydration mismatch on /profile and /share.
  test('does not touch the DOM when the mount effect runs twice on the same path', () => {
    mountPage('<main><h1>Vitrine do jogador</h1></main>');
    render(<StrictMode><RouteAnnouncer/></StrictMode>);
    expect(screen.getByRole('heading')).not.toHaveAttribute('tabindex');
    expect(document.querySelector('main')).not.toHaveAttribute('tabindex');
    expect(screen.getByRole('status')).toHaveTextContent('');
  });

  test('moves focus to the new page heading without a second live-region announcement', () => {
    const page = mountPage('<main><h1>Lobby</h1></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    document.title = 'Mesa de poker · CTech Poker';
    page.innerHTML = '<main><h1>Mesa de poker</h1></main>';
    mocks.pathname = '/table/room-1';
    rerender(<RouteAnnouncer/>);
    expect(screen.getByRole('heading')).toHaveFocus();
    expect(screen.getByRole('heading')).toHaveAttribute('tabindex', '-1');
    // Focusing the heading is the announcement; the live region stays empty so
    // the page name is not read twice.
    expect(screen.getByRole('status')).toHaveTextContent('');
  });

  test('removes the injected tabindex once focus leaves the heading', () => {
    const page = mountPage('<main><h1>Lobby</h1></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    page.innerHTML = '<main><h1>Mesa de poker</h1></main>';
    mocks.pathname = '/table/room-1';
    rerender(<RouteAnnouncer/>);
    const heading = screen.getByRole('heading');
    expect(heading).toHaveAttribute('tabindex', '-1');
    heading.blur();
    expect(heading).not.toHaveAttribute('tabindex');
  });

  test('keeps a pre-existing tabindex on the heading intact after blur', () => {
    const page = mountPage('<main><h1>Lobby</h1></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    page.innerHTML = '<main><h1 tabindex="-1">Mesa de poker</h1></main>';
    mocks.pathname = '/table/room-1';
    rerender(<RouteAnnouncer/>);
    const heading = screen.getByRole('heading');
    heading.blur();
    expect(heading).toHaveAttribute('tabindex', '-1');
  });

  test('falls back to focusing main and announcing the title when the page has no heading', () => {
    mountPage('<main></main>');
    const {rerender} = render(<RouteAnnouncer/>);
    document.title = 'Ranking · CTech Poker';
    mocks.pathname = '/leaderboard';
    rerender(<RouteAnnouncer/>);
    expect(document.querySelector('main')).toHaveFocus();
    expect(screen.getByRole('status')).toHaveTextContent('Ranking · CTech Poker');
  });
});
