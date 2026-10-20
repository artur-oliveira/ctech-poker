import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {UnavailableState} from './UnavailableState';

vi.mock('next/link', () => ({default: ({children, href}: {children: React.ReactNode; href: string}) =>
  <a href={href}>{children}</a>}));

const replace = vi.fn();

describe('UnavailableState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok: false} as Response));
    Object.defineProperty(navigator, 'onLine', {configurable: true, value: true});
    window.sessionStorage.clear();
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {origin: 'http://localhost:3000', href: 'http://localhost:3000/unavailable', pathname: '/unavailable', search: '', replace},
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  test('says the table is back soon and stays put while the API is down', async () => {
    render(<UnavailableState/>);
    expect(screen.getByRole('heading', {level: 1})).toHaveTextContent('A mesa volta em breve.');
    expect(screen.getByRole('alert')).toBeInTheDocument();
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    expect(replace).not.toHaveBeenCalled();
  });

  test('a manual retry probes again', async () => {
    render(<UnavailableState/>);
    await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    await userEvent.click(screen.getByRole('button', {name: 'Tentar novamente'}));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(replace).not.toHaveBeenCalled();
  });

  test('returns to the interrupted route on background recovery, with no tap', async () => {
    window.sessionStorage.setItem('poker:return-after-outage', '/table?id=room-1');
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    render(<UnavailableState/>);
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/table?id=room-1'));
    expect(window.sessionStorage.getItem('poker:return-after-outage')).toBeNull();
  });

  test('falls back to the lobby when no route was saved', async () => {
    vi.mocked(fetch).mockResolvedValue({ok: true} as Response);
    render(<UnavailableState/>);
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/lobby'));
  });
});
