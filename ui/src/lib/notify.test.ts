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
  
  test('bounds notification bursts to the three newest messages', async () => {
    const {pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);
    
    for (let index = 1; index <= 5; index++) {
      pushNotification(`Aviso ${index}`);
    }
    
    expect(listener.mock.lastCall?.[0].map((item: { message: string }) => item.message))
      .toEqual(['Aviso 3', 'Aviso 4', 'Aviso 5']);
  });
  
  test('keeps error toasts past the 6 s timer and clears them at the 20 s ceiling', async () => {
    const {pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);

    pushNotification('Falha ao salvar', 'error');
    vi.advanceTimersByTime(6000);
    expect(listener.mock.lastCall?.[0]).toHaveLength(1);

    vi.advanceTimersByTime(14000);
    expect(listener.mock.lastCall?.[0]).toEqual([]);
  });

  test('does not auto-dismiss an info toast that carries actions on the 6 s timer', async () => {
    const {pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);

    pushNotification('Convite para a mesa', 'info', [{label: 'Entrar', run: () => undefined}]);
    vi.advanceTimersByTime(6000);
    expect(listener.mock.lastCall?.[0]).toHaveLength(1);

    vi.advanceTimersByTime(14000);
    expect(listener.mock.lastCall?.[0]).toEqual([]);
  });

  test('still evicts persistent toasts beyond MAX_VISIBLE', async () => {
    const {pushNotification, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);

    for (let index = 1; index <= 5; index++) pushNotification(`Erro ${index}`, 'error');

    expect(listener.mock.lastCall?.[0].map((item: {message: string}) => item.message))
      .toEqual(['Erro 3', 'Erro 4', 'Erro 5']);
  });

  test.each([
    [{name: 'ApiError', original: {request: {}, message: 'Network Error'}},
      'Sem conexão com o servidor. Verifique sua internet e tente novamente.'],
    [{name: 'ApiError', original: {request: {}, code: 'ECONNABORTED', message: 'timeout of 10000ms exceeded'}},
      'O servidor demorou para responder. Verifique sua conexão e tente novamente.'],
  ])('maps normalized transport failures to an actionable message', async (error, expected) => {
    const {notifyApiError, subscribeNotifications} = await import('./notify');
    const listener = vi.fn();
    subscribeNotifications(listener);
    
    notifyApiError(error);
    
    expect(listener.mock.lastCall?.[0][0]).toMatchObject({message: expected, variant: 'error'});
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
