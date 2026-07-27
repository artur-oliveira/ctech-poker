'use client';
import {useEffect} from 'react';

export function useDealerVoice(message: string, enabled: boolean) {
  useEffect(() => {
    if (!enabled || !message || !('speechSynthesis' in window)) return undefined;
    const speech = new SpeechSynthesisUtterance(message);
    speech.lang = 'pt-BR';
    speech.rate = 1.06;
    speech.pitch = .92;
    window.speechSynthesis.cancel();
    window.speechSynthesis.speak(speech);
    return () => window.speechSynthesis.cancel();
  }, [message, enabled]);
}
