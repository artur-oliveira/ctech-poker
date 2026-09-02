import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, test, vi} from 'vitest';
import type {PokerAction} from '@/lib/api/table';
import {VoiceActionButton} from './VoiceActionButton';

type ResultHandler = ((event: Event & { results: ArrayLike<{ 0: { transcript: string } }> }) => void) | null;

class RecognitionMock {
  static instances: RecognitionMock[] = [];
  lang = '';
  continuous = true;
  interimResults = true;
  start = vi.fn();
  abort = vi.fn();
  onresult: ResultHandler = null;
  onerror: (() => void) | null = null;
  onend: (() => void) | null = null;
  
  constructor() {
    RecognitionMock.instances.push(this);
  }
  
  result(transcript: string) {
    this.onresult?.({results: [{0: {transcript}}]} as never);
  }
}

const available: Record<PokerAction, boolean> = {fold: true, check: true, call: true, raise: true};

function renderButton(overrides: Partial<React.ComponentProps<typeof VoiceActionButton>> = {}) {
  const onAct = vi.fn(() => true);
  const view = render(<VoiceActionButton enabled disabled={false} available={available}
                                         minRaise={200} maxRaise={1_500} onAct={onAct} {...overrides}/>);
  return {onAct, ...view};
}

afterEach(() => {
  delete (window as typeof window & { SpeechRecognition?: unknown }).SpeechRecognition;
  delete (window as typeof window & { webkitSpeechRecognition?: unknown }).webkitSpeechRecognition;
  RecognitionMock.instances = [];
});

describe('VoiceActionButton', () => {
  test('is hidden when disabled by preference or unsupported by the browser', () => {
    const {container, rerender} = render(<VoiceActionButton enabled={false} disabled={false}
                                                            available={available} minRaise={10} maxRaise={100}
                                                            onAct={() => true}/>);
    expect(container).toBeEmptyDOMElement();
    rerender(<VoiceActionButton enabled disabled={false} available={available}
                                minRaise={10} maxRaise={100} onAct={() => true}/>);
    expect(container).toBeEmptyDOMElement();
  });
  
  test('starts recognition with Portuguese settings and stages a raise behind a confirm', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));

    const instance = RecognitionMock.instances[0];
    expect(instance).toMatchObject({lang: 'pt-BR', continuous: false, interimResults: false});
    expect(instance.start).toHaveBeenCalledOnce();
    expect(screen.getByRole('status')).toHaveTextContent('Ouvindo…');

    act(() => instance.result('aumentar'));
    expect(onAct).not.toHaveBeenCalled();
    expect(screen.getByRole('group', {name: 'Confirmar aumento por voz'})).toHaveTextContent('Aumentar para 200');
    expect(screen.getByRole('status')).toHaveTextContent('Diga "confirmar" ou toque para enviar');
  });

  test('a spoken "confirmar" commits the staged raise', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('aumentar para 800'));
    expect(onAct).not.toHaveBeenCalled();

    act(() => RecognitionMock.instances[1].result('confirmar'));
    expect(onAct).toHaveBeenCalledWith('raise', 800);
    expect(screen.getByRole('status')).toHaveTextContent('Comando enviado.');
    expect(screen.queryByRole('group', {name: 'Confirmar aumento por voz'})).toBeNull();
  });

  test('a tap on Confirmar commits the staged all-in', async () => {
    (window as typeof window & { webkitSpeechRecognition: unknown }).webkitSpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('all in'));
    expect(screen.getByRole('group')).toHaveTextContent('All In 1.500');

    await userEvent.click(screen.getByRole('button', {name: 'Confirmar All In 1.500'}));
    expect(onAct).toHaveBeenCalledWith('raise', 1_500);
  });

  test('a voiced raise total is clamped before it is staged', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('aumentar para 5000'));
    await userEvent.click(screen.getByRole('button', {name: 'Confirmar All In 1.500'}));
    expect(onAct).toHaveBeenCalledWith('raise', 1_500);
  });

  test('the staged raise self-cancels after the timeout with no act', async () => {
    vi.useFakeTimers();
    try {
      (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
      const onAct = vi.fn(() => true);
      render(<VoiceActionButton enabled disabled={false} available={available}
                                minRaise={200} maxRaise={1_500} onAct={onAct}/>);
      act(() => screen.getByRole('button', {name: 'Usar comando por voz'}).click());
      act(() => RecognitionMock.instances[0].result('all in'));
      expect(screen.getByRole('group')).toBeInTheDocument();

      act(() => vi.advanceTimersByTime(6_000));
      expect(onAct).not.toHaveBeenCalled();
      expect(screen.queryByRole('group')).toBeNull();
      expect(screen.getByRole('status')).toBeEmptyDOMElement();
    } finally {
      vi.useRealTimers();
    }
  });

  test('the mic button and a spoken "cancelar" drop a staged raise', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('all in'));

    act(() => RecognitionMock.instances[1].result('cancelar'));
    expect(onAct).not.toHaveBeenCalled();
    expect(screen.getByRole('status')).toHaveTextContent('Aumento cancelado.');

    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[2].result('aumentar'));
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar aumento por voz'}));
    expect(screen.queryByRole('group')).toBeNull();
    expect(onAct).not.toHaveBeenCalled();
  });

  test('reports unavailable or unrecognised commands without staging anything', async () => {
    (window as typeof window & { webkitSpeechRecognition: unknown }).webkitSpeechRecognition = RecognitionMock;
    const onAct = vi.fn(() => false);
    renderButton({available: {...available, fold: false}, onAct});
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    const instance = RecognitionMock.instances[0];

    act(() => instance.result('fold'));
    expect(screen.getByRole('status')).toHaveTextContent('Comando não disponível agora.');
    act(() => instance.result('boa mão'));
    expect(screen.getByRole('status')).toHaveTextContent('Comando não disponível agora.');
    expect(screen.queryByRole('group')).toBeNull();
  });

  test('fold, check and call stay single-step', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    for (const [phrase, action] of [['desistir', 'fold'], ['mesa', 'check'], ['pagar', 'call']] as const) {
      await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
      const instance = RecognitionMock.instances.at(-1)!;
      act(() => instance.result(phrase));
      act(() => instance.onend?.());
      expect(onAct).toHaveBeenLastCalledWith(action);
      expect(screen.queryByRole('group')).toBeNull();
    }
  });
  
  test('stops listening, reports recognition errors, and aborts on unmount', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const view = renderButton();
    const button = screen.getByRole('button', {name: 'Usar comando por voz'});
    await userEvent.click(button);
    const first = RecognitionMock.instances[0];
    
    await userEvent.click(screen.getByRole('button', {name: 'Parar comando por voz'}));
    expect(first.abort).toHaveBeenCalledOnce();
    act(() => first.onerror?.());
    expect(screen.getByRole('status')).toHaveTextContent('Não foi possível ouvir.');
    act(() => first.onend?.());
    expect(screen.getByRole('button', {name: 'Usar comando por voz'})).toHaveAttribute('aria-pressed', 'false');
    
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    const second = RecognitionMock.instances[1];
    view.unmount();
    expect(second.abort).toHaveBeenCalledOnce();
  });
  
  test('the confirm chip X button drops the staged raise', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('all in'));
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar aumento'}));
    expect(screen.queryByRole('group')).toBeNull();
    expect(onAct).not.toHaveBeenCalled();
  });

  test('the action control going disabled abandons a staged raise', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const onAct = vi.fn(() => true);
    const {rerender} = render(<VoiceActionButton enabled disabled={false} available={available}
                                                 minRaise={200} maxRaise={1_500} onAct={onAct}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    act(() => RecognitionMock.instances[0].result('all in'));
    expect(screen.getByRole('group')).toBeInTheDocument();

    rerender(<VoiceActionButton enabled disabled available={available}
                                minRaise={200} maxRaise={1_500} onAct={onAct}/>);
    expect(screen.queryByRole('group')).toBeNull();
    expect(onAct).not.toHaveBeenCalled();
  });

  test('does not start while the action control is disabled', () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    renderButton({disabled: true});
    expect(screen.getByRole('button')).toBeDisabled();
    expect(RecognitionMock.instances).toHaveLength(0);
  });
});
