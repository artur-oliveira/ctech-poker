import {act, fireEvent, render as renderBare, screen} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

import {InviteDialog} from './InviteDialog';

// The friends section (see InviteFriends.test.tsx) reads social state, so even
// the link-only dialog now needs a query client.
function render(ui: React.ReactElement) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return renderBare(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({children}: React.PropsWithChildren) => <div>{children}</div>,
  DialogTrigger: ({children, render}: React.PropsWithChildren<{
    render: React.ReactElement<{ ['aria-label']?: string }>;
  }>) => <button type="button" aria-label={render.props['aria-label']}>{children}</button>,
  DialogContent: ({children}: React.PropsWithChildren) => <section>{children}</section>,
  DialogDescription: ({children}: React.PropsWithChildren) => <p>{children}</p>,
  DialogHeader: ({children}: React.PropsWithChildren) => <header>{children}</header>,
  DialogTitle: ({children}: React.PropsWithChildren) => <h2>{children}</h2>,
}));

describe('InviteDialog', () => {
  const url = 'https://poker.example/table/room-7';
  const writeText = vi.fn();
  
  beforeEach(() => {
    vi.useFakeTimers();
    writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {writeText},
    });
    Object.defineProperty(navigator, 'share', {
      configurable: true,
      value: undefined,
    });
  });
  
  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });
  
  test('copies the invitation when native sharing is unavailable and resets its feedback', async () => {
    render(<InviteDialog url={url}/>);
    
    expect(screen.getByRole('button', {name: 'Convidar para a mesa'})).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: 'Convidar para a mesa'})).toBeInTheDocument();
    expect(screen.getByLabelText('Link de convite')).toHaveValue(url);
    
    fireEvent.click(screen.getByRole('button', {name: 'Copiar'}));
    await act(async () => Promise.resolve());
    expect(writeText).toHaveBeenCalledWith(url);
    expect(screen.getByRole('button', {name: 'Copiado'})).toBeInTheDocument();
    
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(screen.getByRole('button', {name: 'Copiar'})).toBeInTheDocument();
  });
  
  test('prefers native sharing without writing to the clipboard', async () => {
    const share = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'share', {configurable: true, value: share});
    render(<InviteDialog url={url}/>);
    
    fireEvent.click(screen.getByRole('button', {name: 'Copiar'}));
    await vi.waitFor(() => expect(share).toHaveBeenCalledWith({url}));
    expect(writeText).not.toHaveBeenCalled();
  });
  
  test('keeps the link available for manual copy when browser APIs reject', async () => {
    const share = vi.fn().mockRejectedValue(new Error('dismissed'));
    Object.defineProperty(navigator, 'share', {configurable: true, value: share});
    render(<InviteDialog url={url}/>);
    
    fireEvent.click(screen.getByRole('button', {name: 'Copiar'}));
    await vi.waitFor(() => expect(share).toHaveBeenCalled());
    expect(screen.getByLabelText('Link de convite')).toHaveValue(url);
    expect(screen.getByRole('button', {name: 'Copiar'})).toBeInTheDocument();
    
    Object.defineProperty(navigator, 'share', {configurable: true, value: undefined});
    writeText.mockRejectedValueOnce(new Error('blocked'));
    fireEvent.click(screen.getByRole('button', {name: 'Copiar'}));
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(url));
    expect(screen.getByRole('button', {name: 'Copiar'})).toBeInTheDocument();
  });
  
  test('selects the complete invitation link on focus', () => {
    render(<InviteDialog url={url}/>);
    const input = screen.getByLabelText('Link de convite') as HTMLInputElement;
    const select = vi.spyOn(input, 'select');
    
    fireEvent.focus(input);
    expect(select).toHaveBeenCalledOnce();
  });
});
