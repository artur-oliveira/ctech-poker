'use client';

import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {Check, Mic, MicOff, X} from 'lucide-react';
import type {PokerAction} from '@/lib/api/table';
import {isVoiceCancellation, isVoiceConfirmation, parseVoiceAction} from '@/lib/voiceActions';

type RecognitionEvent = Event & { results: ArrayLike<{ 0: { transcript: string } }> }
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

// A misheard "tudo" / "all in" would otherwise commit the whole stack with no
// ceremony, so a recognized raise is staged behind an explicit confirm — spoken
// ("confirmar") or tapped — that self-cancels if neither arrives.
const CONFIRM_TIMEOUT_MS = 6_000;

type PendingRaise = { amount: number; label: string };

function constructor() {
  const speechWindow = window as typeof window & {
    SpeechRecognition?: RecognitionConstructor;
    webkitSpeechRecognition?: RecognitionConstructor;
  };
  return speechWindow.SpeechRecognition || speechWindow.webkitSpeechRecognition;
}

const subscribeSupport = () => () => {
};

export function VoiceActionButton({enabled, disabled, available, minRaise, maxRaise, onAct}: {
  enabled: boolean;
  disabled: boolean;
  available: Record<PokerAction, boolean>;
  minRaise: number;
  maxRaise: number;
  onAct: (action: PokerAction, amount?: number) => boolean;
}) {
  const recognition = useRef<Recognition | null>(null);
  const confirmTimer = useRef<number | undefined>(undefined);
  const pendingRef = useRef<PendingRaise | null>(null);
  const supported = useSyncExternalStore(
    subscribeSupport,
    () => Boolean(constructor()),
    () => false
  );
  const [listening, setListening] = useState(false);
  const [feedback, setFeedback] = useState('');
  const [pending, setPending] = useState<PendingRaise | null>(null);

  const clearConfirmTimer = () => {
    if (confirmTimer.current !== undefined) window.clearTimeout(confirmTimer.current);
    confirmTimer.current = undefined;
  };
  const setPendingRaise = (next: PendingRaise | null) => {
    pendingRef.current = next;
    setPending(next);
  };

  useEffect(() => {
    return () => {
      recognition.current?.abort();
      clearConfirmTimer();
    };
  }, []);

  useEffect(() => {
    if (disabled && pendingRef.current) {
      if (confirmTimer.current !== undefined) window.clearTimeout(confirmTimer.current);
      confirmTimer.current = undefined;
      recognition.current?.abort();
      pendingRef.current = null;
      setPending(null);
      setFeedback('');
    }
  }, [disabled]);

  if (!enabled || !supported) return null;

  const runRecognition = (handle: (transcript: string) => void, waiting: string) => {
    const RecognitionAPI = constructor();
    if (!RecognitionAPI || disabled) return;
    recognition.current?.abort();
    const next = new RecognitionAPI();
    next.lang = 'pt-BR';
    next.continuous = false;
    next.interimResults = false;
    next.onresult = event => handle(event.results[0]?.[0]?.transcript || '');
    next.onerror = () => setFeedback('Não foi possível ouvir.');
    next.onend = () => setListening(false);
    recognition.current = next;
    setFeedback(waiting);
    setListening(true);
    next.start();
  };

  const cancelPending = (announce: boolean) => {
    clearConfirmTimer();
    recognition.current?.abort();
    setListening(false);
    setPendingRaise(null);
    setFeedback(announce ? 'Aumento cancelado.' : '');
  };

  const commitPending = () => {
    const current = pendingRef.current;
    if (!current) return;
    clearConfirmTimer();
    recognition.current?.abort();
    setListening(false);
    setPendingRaise(null);
    setFeedback(onAct('raise', current.amount) ? 'Comando enviado.' : 'Comando não disponível agora.');
  };

  const handleConfirm = (transcript: string) => {
    if (isVoiceConfirmation(transcript)) commitPending();
    else if (isVoiceCancellation(transcript)) cancelPending(true);
    // Anything else leaves the confirm chip up for a tap or the timeout.
  };

  const stageRaise = (amount: number, allIn: boolean) => {
    const label = allIn
      ? `All In ${amount.toLocaleString('pt-BR')}`
      : `Aumentar para ${amount.toLocaleString('pt-BR')}`;
    setPendingRaise({amount, label});
    clearConfirmTimer();
    confirmTimer.current = window.setTimeout(() => cancelPending(false), CONFIRM_TIMEOUT_MS);
    runRecognition(handleConfirm, `${label}. Diga "confirmar" ou toque para enviar.`);
  };

  const handleCommand = (transcript: string) => {
    const command = parseVoiceAction(transcript);
    if (!command || !available[command.action]) {
      setFeedback('Comando não disponível agora.');
      return;
    }
    if (command.action === 'raise') {
      const amount = command.allIn
        ? maxRaise
        : command.amount ? Math.min(maxRaise, Math.max(minRaise, command.amount)) : minRaise;
      stageRaise(amount, Boolean(command.allIn) || amount >= maxRaise);
      return;
    }
    if (onAct(command.action)) setFeedback('Comando enviado.');
  };

  const onMicClick = () => {
    if (pending) cancelPending(true);
    else if (listening) recognition.current?.abort();
    else runRecognition(handleCommand, 'Ouvindo…');
  };

  return <div className="voice-action">
    <button type="button" disabled={disabled} aria-pressed={listening}
            aria-label={pending ? 'Cancelar aumento por voz' : listening ? 'Parar comando por voz' : 'Usar comando por voz'}
            title="Diga Fold, Check, Pagar, Aumentar ou All In"
            onClick={onMicClick}>
      {listening || pending ? <MicOff aria-hidden="true"/> : <Mic aria-hidden="true"/>}
    </button>
    {pending && <div className="voice-confirm" role="group" aria-label="Confirmar aumento por voz">
      <span className="voice-confirm-label">{pending.label}</span>
      <button type="button" className="voice-confirm-yes" disabled={disabled}
              aria-label={`Confirmar ${pending.label}`} onClick={commitPending}>
        <Check aria-hidden="true"/> Confirmar
      </button>
      <button type="button" className="voice-confirm-no"
              aria-label="Cancelar aumento" onClick={() => cancelPending(true)}>
        <X aria-hidden="true"/>
      </button>
    </div>}
    <span className="sr-only" role="status" aria-live="polite">{feedback}</span>
  </div>;
}
