import {createContext, useContext} from 'react';
import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

import {TablePreferencesDialog} from './TablePreferencesDialog';

const {useTablePreferences, update} = vi.hoisted(() => ({
  useTablePreferences: vi.fn(),
  update: vi.fn(),
}));

vi.mock('@/lib/tablePreferences', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/tablePreferences')>(),
  useTablePreferences,
}));

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({children}: React.PropsWithChildren) => <div>{children}</div>,
  DialogTrigger: ({children, render}: React.PropsWithChildren<{
    render?: React.ReactElement<{ ['aria-label']?: string }>;
  }>) => <button type="button" aria-label={render?.props['aria-label']}>{children}</button>,
  DialogContent: ({children}: React.PropsWithChildren) => <section>{children}</section>,
  DialogHeader: ({children}: React.PropsWithChildren) => <header>{children}</header>,
  DialogTitle: ({children}: React.PropsWithChildren) => <h2>{children}</h2>,
  DialogDescription: ({children}: React.PropsWithChildren) => <p>{children}</p>,
}));

vi.mock('@/components/ui/select', () => {
  const ChangeContext = createContext<(value: string | null) => void>(() => undefined);
  return {
    Select: ({children, onValueChange}: React.PropsWithChildren<{ onValueChange: (value: never) => void }>) =>
      <ChangeContext.Provider value={onValueChange as unknown as (value: string | null) => void}>
        {children}
      </ChangeContext.Provider>,
    SelectTrigger: ({children, ...props}: React.PropsWithChildren) => <div {...props}>{children}</div>,
    SelectValue: ({children}: { children: (value: never) => React.ReactNode }) =>
      <>{children('classic' as never)}</>,
    SelectContent: ({children}: React.PropsWithChildren) => <div>{children}</div>,
    SelectItem: ({children, value}: React.PropsWithChildren<{ value: string }>) => {
      const change = useContext(ChangeContext);
      return <button type="button" onClick={() => change(value)}>{children}</button>;
    },
  };
});

vi.mock('@/components/ui/switch', () => ({
  Switch: ({checked, onCheckedChange, ...props}: {
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
    [key: string]: unknown;
  }) => <button type="button" role="switch" aria-checked={checked}
                onClick={() => onCheckedChange(!checked)} {...props}/>,
}));

describe('TablePreferencesDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useTablePreferences.mockReturnValue({
      preferences: {
        theme: 'classic',
        dealerVoice: false,
        voiceCommands: true,
        realityCheckMinutes: 60,
      },
      update,
    });
  });
  
  test('renders the current preferences and all supported choices', () => {
    render(<TablePreferencesDialog/>);
    
    expect(screen.getByRole('button', {name: 'Preferências da mesa'})).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: 'Preferências da mesa'})).toBeInTheDocument();
    expect(screen.getByText('Personalize a experiência e escolha como prefere jogar nesta mesa.')).toBeInTheDocument();
    expect(screen.getAllByText('Clássico').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', {name: 'Meia-noite'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Bordô'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Oceano'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Desativado'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'A cada 2 horas'})).toBeInTheDocument();
    expect(screen.getAllByRole('switch')).toHaveLength(2);
    expect(screen.getAllByRole('switch')[0]).toHaveAttribute('aria-checked', 'false');
    expect(screen.getAllByRole('switch')[1]).toHaveAttribute('aria-checked', 'true');
  });
  
  test('shows the run-it-twice choice only when the room allows it', async () => {
    const onChange = vi.fn(() => true);
    const hidden = render(<TablePreferencesDialog/>);
    expect(screen.queryByText('Rodar duas vezes')).not.toBeInTheDocument();
    hidden.unmount();
    
    render(<TablePreferencesDialog runItTwiceAvailable runItTwice={false}
                                   onRunItTwiceChange={onChange}/>);
    expect(screen.getByText(/todos os jogadores envolvidos também ativaram/)).toBeInTheDocument();
    const toggle = screen.getByRole('switch', {name: 'Rodar duas vezes'});
    await userEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(true);
  });
  
  test('updates theme, voice settings, and session reminder independently', async () => {
    render(<TablePreferencesDialog/>);
    
    await userEvent.click(screen.getByRole('button', {name: 'Meia-noite'}));
    await userEvent.click(screen.getAllByRole('switch')[0]);
    await userEvent.click(screen.getAllByRole('switch')[1]);
    await userEvent.click(screen.getByRole('button', {name: 'A cada 30 minutos'}));
    
    expect(update).toHaveBeenCalledWith({theme: 'midnight'});
    expect(update).toHaveBeenCalledWith({dealerVoice: true});
    expect(update).toHaveBeenCalledWith({voiceCommands: false});
    expect(update).toHaveBeenCalledWith({realityCheckMinutes: 30});
    expect(update).toHaveBeenCalledTimes(4);
  });
});
