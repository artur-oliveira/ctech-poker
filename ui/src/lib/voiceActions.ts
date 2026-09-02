import type {PokerAction} from '@/lib/api/table';

export type VoiceAction = { action: PokerAction; allIn?: boolean; amount?: number };

// Speech-to-text renders spoken numbers as digits ("aumentar para 500"), so a
// digit run is enough to recover a specific voiced total; word-form numbers
// ("quinhentos") aren't handled, that's a bigger ask than this covers.
function parseSpokenAmount(normalized: string): number | undefined {
  const digits = normalized.match(/\d[\d.,]*/)?.[0].replace(/[.,]/g, '');
  const amount = digits ? Number(digits) : undefined;
  return amount && Number.isFinite(amount) ? amount : undefined;
}

function normalizeTranscript(transcript: string): string {
  return transcript.toLocaleLowerCase('pt-BR').normalize('NFD').replace(/\p{Diacritic}/gu, '').trim();
}

// A raise / all-in is the one irreversible chip commitment reachable by voice, so
// it is staged behind a spoken (or tapped) confirmation. STT drops the "-mar"
// ending often enough that "confirma" has to count too.
export function isVoiceConfirmation(transcript: string): boolean {
  return /\b(confirm(ar|a|o|ado)?|isso|pode ir)\b/.test(normalizeTranscript(transcript));
}

export function isVoiceCancellation(transcript: string): boolean {
  return /\b(cancelar|cancela|cancelo|nao|para|parar|esquec[ae])\b/.test(normalizeTranscript(transcript));
}

export function parseVoiceAction(transcript: string): VoiceAction | null {
  const normalized = normalizeTranscript(transcript);
  if (/\b(all ?in|tudo)\b/.test(normalized)) return {action: 'raise', allIn: true};
  if (/\b(aumentar|aumento|raise)\b/.test(normalized)) {
    const amount = parseSpokenAmount(normalized);
    return amount ? {action: 'raise', amount} : {action: 'raise'};
  }
  if (/\b(pagar|pago|call)\b/.test(normalized)) return {action: 'call'};
  if (/\b(check|mesa|passar|passo)\b/.test(normalized)) return {action: 'check'};
  if (/\b(fold|desistir|desisto)\b/.test(normalized)) return {action: 'fold'};
  return null;
}
