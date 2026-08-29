import {act, fireEvent, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

import {pushNotification} from '@/lib/notify';
import {Notifier} from './Notifier';

describe('Notifier', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-28T12:00:00Z'));
  });
  
  afterEach(() => {
    vi.useRealTimers();
  });
  
  test('renders queued notices with live-region semantics and closes one independently', () => {
    render(<Notifier/>);
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
    
    act(() => {
      pushNotification('Servidor indisponível');
      vi.advanceTimersByTime(601);
      pushNotification('Manutenção programada', 'info');
    });
    
    expect(screen.getByRole('region', {name: 'Avisos'})).not.toHaveAttribute('aria-live');
    expect(screen.getByRole('alert')).toHaveTextContent('Servidor indisponível');
    expect(screen.getByRole('status')).toHaveTextContent('Manutenção programada');
    fireEvent.click(screen.getAllByRole('button', {name: 'Fechar aviso'})[0]);
    
    expect(screen.queryByText('Servidor indisponível')).not.toBeInTheDocument();
    expect(screen.getByText('Manutenção programada')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Fechar aviso'}));
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
  });
  
  test('runs a notification action and dismisses the toast', async () => {
    const run = vi.fn();
    render(<Notifier/>);
    act(() => pushNotification('Convite de mesa', 'info', [{label: 'Entrar', run}]));

    fireEvent.click(screen.getByRole('button', {name: 'Entrar'}));
    expect(run).toHaveBeenCalledOnce();
    expect(screen.queryByText('Convite de mesa')).not.toBeInTheDocument();
  });

  test('removes an alert after its automatic timeout', () => {
    render(<Notifier/>);
    act(() => pushNotification('Aviso passageiro'));
    expect(screen.getByText('Aviso passageiro')).toBeInTheDocument();
    
    act(() => vi.advanceTimersByTime(6000));
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
  });
});
