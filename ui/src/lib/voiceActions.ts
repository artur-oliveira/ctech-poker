import type {PokerAction} from '@/lib/api/table';

export type VoiceAction = {action: PokerAction; allIn?: boolean};

export function parseVoiceAction(transcript: string): VoiceAction | null {
  const normalized = transcript.toLocaleLowerCase('pt-BR').normalize('NFD').replace(/\p{Diacritic}/gu, '').trim();
  if (/\b(all ?in|tudo)\b/.test(normalized)) return {action: 'raise', allIn: true};
  if (/\b(aumentar|aumento|raise)\b/.test(normalized)) return {action: 'raise'};
  if (/\b(pagar|pago|call)\b/.test(normalized)) return {action: 'call'};
  if (/\b(check|mesa|passar|passo)\b/.test(normalized)) return {action: 'check'};
  if (/\b(fold|desistir|desisto)\b/.test(normalized)) return {action: 'fold'};
  return null;
}
