import {act, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {BotChallenge} from './BotChallenge';

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
  });
});
