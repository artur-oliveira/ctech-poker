'use client';

import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {Mic, MicOff} from 'lucide-react';
import type {PokerAction} from '@/lib/api/table';
import {parseVoiceAction} from '@/lib/voiceActions';

type RecognitionEvent = Event & {results: ArrayLike<{0: {transcript: string}}>}
type Recognition = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  start: () => void;
  abort: () => void;
  onresult: ((event: RecognitionEvent) => void) | null;
  onerror: (() => void) | null;
  onend: (() => void) | null;
}
type RecognitionConstructor = new () => Recognition;

function constructor() {
  const speechWindow = window as typeof window & {
    SpeechRecognition?: RecognitionConstructor;
    webkitSpeechRecognition?: RecognitionConstructor;
  };
  return speechWindow.SpeechRecognition || speechWindow.webkitSpeechRecognition;
}

const subscribeSupport = () => () => {};

export function VoiceActionButton({enabled, disabled, available, minRaise, maxRaise, onAct}: {
  enabled: boolean;
  disabled: boolean;
  available: Record<PokerAction, boolean>;
  minRaise: number;
  maxRaise: number;
  onAct: (action: PokerAction, amount?: number) => boolean;
}) {
  const recognition = useRef<Recognition | null>(null);
  const supported = useSyncExternalStore(
    subscribeSupport,
    () => Boolean(constructor()),
    () => false
  );
  const [listening, setListening] = useState(false);
  const [feedback, setFeedback] = useState('');

  useEffect(() => {
    return () => recognition.current?.abort();
  }, []);

  if (!enabled || !supported) return null;
  const listen = () => {
    const RecognitionAPI = constructor();
    if (!RecognitionAPI || disabled) return;
    recognition.current?.abort();
    const next = new RecognitionAPI();
    next.lang = 'pt-BR';
    next.continuous = false;
    next.interimResults = false;
    next.onresult = event => {
      const command = parseVoiceAction(event.results[0]?.[0]?.transcript || '');
      if (!command || !available[command.action]) {
        setFeedback('Comando não disponível agora.');
        return;
      }
      const amount = command.action === 'raise' ? command.allIn ? maxRaise : minRaise : undefined;
      if (onAct(command.action, amount)) setFeedback('Comando enviado.');
    };
    next.onerror = () => setFeedback('Não foi possível ouvir.');
    next.onend = () => setListening(false);
    recognition.current = next;
    setFeedback('Ouvindo…');
    setListening(true);
    next.start();
  };
  return <div className="voice-action">
    <button type="button" disabled={disabled} aria-pressed={listening}
            aria-label={listening ? 'Parar comando por voz' : 'Usar comando por voz'}
            title="Diga Fold, Check, Pagar, Aumentar ou All In"
            onClick={() => listening ? recognition.current?.abort() : listen()}>
      {listening ? <MicOff aria-hidden="true"/> : <Mic aria-hidden="true"/>}
    </button>
    <span className="sr-only" role="status" aria-live="polite">{feedback}</span>
  </div>;
}
