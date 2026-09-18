import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {BotChallenge} from './BotChallenge';
import {expectNoAxeViolations} from '@/test/axe';

const {fileBotCheckContest} = vi.hoisted(() => ({fileBotCheckContest: vi.fn()}));
vi.mock('@/lib/api/botCheckContest', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/botCheckContest')>(), fileBotCheckContest,
}));

vi.mock('next/link', () => ({
  default: ({href, children}: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({children}: { children: React.ReactNode }) => <>{children}</>,
  DialogContent: ({children}: { children: React.ReactNode }) => <section>{children}</section>,
  DialogHeader: ({children}: { children: React.ReactNode }) => <header>{children}</header>,
  DialogTitle: ({children}: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({children}: { children: React.ReactNode }) => <p>{children}</p>,
}));

type Options = {
  callback: (token: string) => void;
  'error-callback': () => void;
  'expired-callback': () => void;
};

const originalLocation = window.location;

beforeEach(() => {
  fileBotCheckContest.mockReset();
});

afterEach(() => {
  vi.unstubAllEnvs();
  document.getElementById('cloudflare-turnstile-script')?.remove();
  delete (window as typeof window & { turnstile?: unknown }).turnstile;
  Object.defineProperty(window, 'location', {value: originalLocation, writable: true});
});

describe('BotChallenge', () => {
  test('stays hidden when verification is not required', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
    const {container} = render(<BotChallenge required={false} onTokenAction={() => true}/>);
    expect(container).toBeEmptyDOMElement();
    expect(document.getElementById('cloudflare-turnstile-script')).toBeNull();
  });
  
  test('explains a missing site key without loading the external script, and offers a way out', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', '');
    const reload = vi.fn();
    Object.defineProperty(window, 'location', {value: {...window.location, reload}, writable: true});
    render(<BotChallenge required onTokenAction={() => true}/>);
    expect(screen.getByRole('alert')).toHaveTextContent('não foi configurada');
    expect(document.getElementById('cloudflare-turnstile-script')).toBeNull();
    fireEvent.click(screen.getByRole('button', {name: 'Recarregar página'}));
    expect(reload).toHaveBeenCalledOnce();
    expect(screen.getByRole('link', {name: 'Voltar ao lobby'})).toHaveAttribute('href', '/lobby');
  });

  test('reaches an error state with reload + lobby exit when the script never loads', () => {
    vi.useFakeTimers();
    try {
      vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
      const reload = vi.fn();
      Object.defineProperty(window, 'location', {value: {...window.location, reload}, writable: true});
      render(<BotChallenge required onTokenAction={() => true}/>);
      expect(screen.getByText('Preparando verificação…')).toBeInTheDocument();
      expect(screen.queryByRole('alert')).toBeNull();

      act(() => {
        vi.advanceTimersByTime(15_000);
      });

      expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível validar');
      fireEvent.click(screen.getByRole('button', {name: 'Recarregar página'}));
      expect(reload).toHaveBeenCalledOnce();
      expect(screen.getByRole('link', {name: 'Voltar ao lobby'})).toHaveAttribute('href', '/lobby');
    } finally {
      vi.useRealTimers();
    }
  });

  test('offers reload + lobby exit while still loading, before the timeout', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
    render(<BotChallenge required onTokenAction={() => true}/>);
    expect(screen.getByRole('button', {name: 'Recarregar página'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Voltar ao lobby'})).toBeInTheDocument();
  });

  test('a ready widget hides the recovery controls', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
    (window as typeof window & { turnstile: unknown }).turnstile = {
      render: () => 'widget-ready',
      remove: vi.fn(),
    };
    render(<BotChallenge required onTokenAction={() => true}/>);
    fireEvent.load(document.getElementById('cloudflare-turnstile-script')!);
    expect(screen.queryByRole('button', {name: 'Recarregar página'})).toBeNull();
    expect(screen.queryByRole('link', {name: 'Voltar ao lobby'})).toBeNull();
  });
  
  test('loads Turnstile, validates a token and removes its widget on cleanup', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
    let options: Options | undefined;
    const renderWidget = vi.fn((_element, received: Options) => {
      options = received;
      return 'widget-1';
    });
    const remove = vi.fn();
    (window as typeof window & { turnstile: unknown }).turnstile = {render: renderWidget, remove};
    const onTokenAction = vi.fn(() => true);
    
    const view = render(<BotChallenge required onTokenAction={onTokenAction}/>);
    const script = document.getElementById('cloudflare-turnstile-script') as HTMLScriptElement;
    expect(script.src).toContain('challenges.cloudflare.com/turnstile');
    fireEvent.load(script);
    
    expect(renderWidget).toHaveBeenCalledOnce();
    expect(options).toMatchObject({sitekey: 'site-key', action: 'poker_bot_check', language: 'pt-BR'});
    act(() => options?.callback('verified-token'));
    expect(onTokenAction).toHaveBeenCalledWith('verified-token');
    expect(screen.getByText('Validando…')).toBeInTheDocument();
    
    view.unmount();
    expect(remove).toHaveBeenCalledWith('widget-1');
  });
  
  test('shows failures from script, provider, expiration and rejected tokens', () => {
    vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
    const onTokenAction = vi.fn(() => false);
    const view = render(<BotChallenge required onTokenAction={onTokenAction}/>);
    fireEvent.error(document.getElementById('cloudflare-turnstile-script')!);
    expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível validar');
    view.unmount();
    
    let options: Options | undefined;
    (window as typeof window & { turnstile: unknown }).turnstile = {
      render: (_element: HTMLElement, received: Options) => {
        options = received;
        return 'widget-2';
      },
      remove: vi.fn(),
    };
    render(<BotChallenge required onTokenAction={onTokenAction}/>);
    fireEvent.load(document.getElementById('cloudflare-turnstile-script')!);
    act(() => options?.callback('bad-token'));
    expect(onTokenAction).toHaveBeenCalledWith('bad-token');
    expect(screen.getByRole('alert')).toBeInTheDocument();
    act(() => options?.['error-callback']());
    act(() => options?.['expired-callback']());
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Recarregar página'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Voltar ao lobby'})).toBeInTheDocument();
  });

  describe('contest flow (#322)', () => {
    function reachErrorState(onTokenAction = vi.fn(() => false)) {
      const view = render(<BotChallenge required onTokenAction={onTokenAction} tableId="table-1"/>);
      fireEvent.error(document.getElementById('cloudflare-turnstile-script')!);
      return {onTokenAction, container: view.container};
    }

    test('offers a contesting path only once the player is actually blocked, never before', () => {
      vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
      render(<BotChallenge required onTokenAction={() => true} tableId="table-1"/>);
      expect(screen.queryByRole('button', {name: /Conte pra gente/})).toBeNull();
    });

    test('lets a blocked player file an auditable contest, distinct from retrying verification', async () => {
      vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
      fileBotCheckContest.mockResolvedValue({contest_id: 'c-1', status: 'pending', created_at: 't'});
      const user = userEvent.setup();
      const {container} = reachErrorState();

      await user.click(screen.getByRole('button', {name: /Conte pra gente/}));
      await expectNoAxeViolations(container);
      await user.type(screen.getByLabelText('O que aconteceu (opcional)'), 'joguei rápido de propósito');
      await user.click(screen.getByRole('button', {name: 'Enviar contestação'}));

      expect(await screen.findByRole('status')).toHaveTextContent('aguardando revisão');
      expect(fileBotCheckContest).toHaveBeenCalledWith({tableId: 'table-1', reason: 'joguei rápido de propósito'});
      // Filing the contest never re-runs or short-circuits Turnstile's own
      // verification — it is a separate append-only trail (#322).
      expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível validar');
    });

    test('reports a failed contest submission without losing the player\'s draft', async () => {
      vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
      fileBotCheckContest.mockRejectedValue(new Error('network'));
      const user = userEvent.setup();
      reachErrorState();

      await user.click(screen.getByRole('button', {name: /Conte pra gente/}));
      await user.click(screen.getByRole('button', {name: 'Enviar contestação'}));

      await waitFor(() => {
        expect(screen.getAllByRole('alert').some(node => node.textContent?.includes('Não foi possível enviar')))
          .toBe(true);
      });
      // The form stays open and untouched by the failure — nothing was lost.
      expect(screen.getByRole('button', {name: 'Enviar contestação'})).toBeInTheDocument();
    });

    test('cancel closes the form without ever filing a contest', async () => {
      vi.stubEnv('NEXT_PUBLIC_TURNSTILE_SITE_KEY', 'site-key');
      const user = userEvent.setup();
      reachErrorState();

      await user.click(screen.getByRole('button', {name: /Conte pra gente/}));
      await user.click(screen.getByRole('button', {name: 'Cancelar'}));

      expect(screen.getByRole('button', {name: /Conte pra gente/})).toBeInTheDocument();
      expect(fileBotCheckContest).not.toHaveBeenCalled();
    });
  });
});
