export type Suit = 'spade' | 'heart' | 'diamond' | 'club';

export type DeckVariantId = (
  'four-color' |
  'two-color' |
  'four-color-alt-1' |
  'four-color-alt-2' |
  'colorblind' |
  'casino' |
  'neon' |
  'pastel' |
  'cyber' |
  'royal' |
  'candy' |
  'mono' |
  'solarized' |
  'synthwave' |
  'material' |
  'vibrant' |
  'retro'
  )

export interface DeckVariant {
  label: string;
  colors: Record<Suit, string>;
}

export const DECK_VARIANTS: Record<DeckVariantId, DeckVariant> = {
  'four-color': {
    label: 'Quatro cores',
    colors: {
      spade: '#1F2937',
      heart: '#DC2626',
      diamond: '#2563EB',
      club: '#16A34A'
    }
  },
  'two-color': {
    label: 'Duas cores',
    colors: {
      spade: '#1F2937',
      club: '#1F2937',
      heart: '#DC2626',
      diamond: '#DC2626'
    }
  },
  'four-color-alt-1': {
    label: 'Quatro cores #1',
    colors: {
      spade: '#222222',
      heart: '#E53935',
      diamond: '#2979FF',
      club: '#2E7D32'
    }
  },
  'four-color-alt-2': {
    label: 'Quatro cores #2',
    colors: {
      spade: '#202124',
      heart: '#C62828',
      diamond: '#1565C0',
      club: '#2E7D32'
    }
  },
  colorblind: {
    // Okabe-Ito colorblind-safe palette
    label: 'Daltonismo',
    colors: {
      spade: '#000000',
      heart: '#D55E00',
      diamond: '#0072B2',
      club: '#009E73'
    }
  },
  casino: {
    label: 'Cassino',
    colors: {
      spade: '#111827',
      heart: '#B91C1C',
      diamond: '#B91C1C',
      club: '#111827'
    }
  },
  neon: {
    label: 'Neon',
    colors: {
      spade: '#7C3AED',
      heart: '#F43F5E',
      diamond: '#0EA5E9',
      club: '#22C55E'
    }
  },
  pastel: {
    label: 'Pastel',
    colors: {
      spade: '#5C6370',
      heart: '#FF6B81',
      diamond: '#74C0FC',
      club: '#69DB7C'
    }
  },
  cyber: {
    label: 'Cyber',
    colors: {
      spade: '#A855F7',
      heart: '#EF4444',
      diamond: '#06B6D4',
      club: '#10B981'
    }
  },
  royal: {
    label: 'Royal',
    colors: {
      spade: '#1E3A8A',
      heart: '#991B1B',
      diamond: '#D97706',
      club: '#065F46'
    }
  },
  candy: {
    label: 'Candy',
    colors: {
      spade: '#6D28D9',
      heart: '#EC4899',
      diamond: '#3B82F6',
      club: '#22C55E'
    }
  },
  mono: {
    label: 'Monocromático',
    colors: {
      spade: '#374151',
      heart: '#6B7280',
      diamond: '#9CA3AF',
      club: '#D1D5DB'
    }
  },
  solarized: {
    label: 'Solarized',
    colors: {
      spade: '#073642',
      heart: '#DC322F',
      diamond: '#268BD2',
      club: '#859900'
    }
  },
  synthwave: {
    label: 'Synthwave',
    colors: {
      spade: '#8B5CF6',
      heart: '#F43F5E',
      diamond: '#38BDF8',
      club: '#14B8A6'
    }
  },
  material: {
    label: 'Material',
    colors: {
      spade: '#374151',
      heart: '#EF4444',
      diamond: '#3B82F6',
      club: '#10B981'
    }
  },
  vibrant: {
    label: 'Vibrante',
    colors: {
      spade: '#6D28D9',
      heart: '#E11D48',
      diamond: '#0284C7',
      club: '#15803D'
    }
  },
  retro: {
    label: 'Retrô',
    colors: {
      spade: '#4B5563',
      heart: '#C2410C',
      diamond: '#1D4ED8',
      club: '#4D7C0F'
    }
  }

};

export const DEFAULT_DECK_VARIANT: DeckVariantId = 'four-color';
