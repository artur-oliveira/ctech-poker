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
  
  test('starts recognition with Portuguese settings and sends a regular raise', async () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    const {onAct} = renderButton();
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    
    const instance = RecognitionMock.instances[0];
    expect(instance).toMatchObject({lang: 'pt-BR', continuous: false, interimResults: false});
    expect(instance.start).toHaveBeenCalledOnce();
    expect(screen.getByRole('status')).toHaveTextContent('Ouvindo…');
    
    act(() => instance.result('aumentar'));
    expect(onAct).toHaveBeenCalledWith('raise', 200);
    expect(screen.getByRole('status')).toHaveTextContent('Comando enviado.');
  });
  
  test('uses maximum raise for all-in and reports unavailable or rejected commands', async () => {
    (window as typeof window & { webkitSpeechRecognition: unknown }).webkitSpeechRecognition = RecognitionMock;
    const onAct = vi.fn(() => false);
    renderButton({available: {...available, fold: false}, onAct});
    await userEvent.click(screen.getByRole('button', {name: 'Usar comando por voz'}));
    const instance = RecognitionMock.instances[0];
    
    act(() => instance.result('all in'));
    expect(onAct).toHaveBeenCalledWith('raise', 1_500);
    expect(screen.getByRole('status')).toHaveTextContent('Ouvindo…');
    act(() => instance.result('fold'));
    expect(screen.getByRole('status')).toHaveTextContent('Comando não disponível agora.');
    act(() => instance.result('boa mão'));
    expect(screen.getByRole('status')).toHaveTextContent('Comando não disponível agora.');
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
  
  test('does not start while the action control is disabled', () => {
    (window as typeof window & { SpeechRecognition: unknown }).SpeechRecognition = RecognitionMock;
    renderButton({disabled: true});
    expect(screen.getByRole('button')).toBeDisabled();
    expect(RecognitionMock.instances).toHaveLength(0);
  });
});
