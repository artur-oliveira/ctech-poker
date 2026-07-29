import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

describe('notification store', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-28T12:00:00Z'));
    vi.resetModules();
  });
  
  afterEach(() => {
    vi.useRealTimers();
  });
  
  test('publishes, deduplicates and automatically dismisses notifications', async () => {
    const {pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    const unsubscribe = subscribeNotifications(listener);
    
    pushNotification('Falha temporária', 'info');
    pushNotification('Falha temporária', 'info');
    
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener.mock.lastCall?.[0]).toEqual([
      expect.objectContaining({message: 'Falha temporária', variant: 'info'}),
    ]);
    
    vi.advanceTimersByTime(6000);
    expect(listener.mock.lastCall?.[0]).toEqual([]);
    
    unsubscribe();
    vi.advanceTimersByTime(601);
    pushNotification('Outro aviso');
    expect(listener).toHaveBeenCalledTimes(2);
  });
  
  test('allows explicit dismissal and a repeated message after the dedupe window', async () => {
    const {dismissNotification, pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);
    
    pushNotification('Repita');
    const first = listener.mock.lastCall?.[0][0];
    dismissNotification(first.id);
    expect(listener.mock.lastCall?.[0]).toEqual([]);
    
    vi.advanceTimersByTime(600);
    pushNotification('Repita');
    expect(listener.mock.lastCall?.[0]).toHaveLength(1);
  });
  
  test.each([
    [{}, 'Algo deu errado. Tente novamente.'],
    [{isAxiosError: true, request: {}}, 'Sem conexão com o servidor. Verifique sua internet e tente novamente.'],
    [{isAxiosError: true, response: {status: 401}}, 'Sua sessão expirou. Entre novamente para continuar.'],
    [{
      isAxiosError: true,
      response: {status: 503}
    }, 'O servidor falhou ao processar a solicitação. Tente novamente em alguns instantes.'],
    [{isAxiosError: true, response: {status: 418}}, 'Algo deu errado. Tente novamente.'],
  ])('maps API failures to a safe user message', async (error, expected) => {
    const {notifyApiError, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);
    
    notifyApiError(error);
    
    expect(listener.mock.lastCall?.[0][0]).toMatchObject({message: expected, variant: 'error'});
  });
});
