export const PLAYSTYLE = {
  selective: {label: 'Seletivo', reason: 'VPIP de até 22%'},
  explorer: {label: 'Explorador', reason: 'VPIP a partir de 38%'},
  initiative: {label: 'Iniciativa', reason: 'PFR representa pelo menos 70% do VPIP'},
  counter: {label: 'Contra-ataque', reason: '3-bet de pelo menos 10%'},
  balanced: {label: 'Equilibrado', reason: 'Sem tendência dominante nesta amostra'},
} as const;

export type PlaystyleKey = keyof typeof PLAYSTYLE;
export type PlaystyleBadge = {key: string; label?: string; reason?: string};

export function playstyleMeta(key: string) {
  return PLAYSTYLE[key as PlaystyleKey];
}

