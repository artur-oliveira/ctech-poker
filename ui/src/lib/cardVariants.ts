export type Suit = 'spade' | 'heart' | 'diamond' | 'club';

export type DeckVariantId = (
  'two-color' |
  'four-color' |
  'colorblind' |
  'high-constrast' |
  'casino' |
  'bicycle' |
  'vintage' |
  'golden' |
  'pink' |
  'alt'
  )

export interface DeckVariant {
  label: string;
  colors: Record<Suit, string>;
}

export const DECK_VARIANTS: Record<DeckVariantId, DeckVariant> = {
  'four-color': {
    label: 'Padrão',
    colors: {
      spade: '#1A1A1A',
      heart: '#E60000',
      diamond: '#0066CC',
      club: '#008A00'
    }
  },
  'two-color': {
    label: 'Clássico',
    colors: {
      spade: '#1A1A1A',
      club: '#1A1A1A',
      heart: '#CC0000',
      diamond: '#CC0000'
    }
  },
  colorblind: {
    label: 'Daltonismo',
    colors: {
      spade: '#000000',
      heart: '#E69F00',
      diamond: '#56B4E9',
      club: '#009E73'
    }
  },
  'high-constrast': {
    label: 'Contraste',
    colors: {
      spade: '#000000',
      heart: '#FF0000',
      diamond: '#0055FF',
      club: '#00AA00'
    }
  },
  casino: {
    label: 'Cassino',
    colors: {
      spade: '#1A1A2E',
      heart: '#8B0000',
      diamond: '#8B0000',
      club: '#1A1A2E'
    }
  },
  bicycle: {
    label: 'Bicycle',
    colors: {
      spade: '#1A1A1A',
      heart: '#B22222',
      diamond: '#B22222',
      club: '#1A1A1A'
    }
  },
  vintage: {
    label: 'Vintage',
    colors: {
      spade: '#3D2B1F',
      heart: '#8B3A3A',
      diamond: '#2C5282',
      club: '#276749'
    }
  },
  golden: {
    label: 'Dourado',
    colors: {
      spade: '#141414',
      heart: '#8B0000',
      diamond: '#C9A227',
      club: '#0E3B2E'
    }
  },
  pink: {
    label: 'Rosa',
    colors: {
      spade: '#2E1A2E',
      heart: '#FF4D6D',
      diamond: '#FF8FB1',
      club: '#6B2D5C'
    }
  },
  alt: {
    label: 'Alternativo',
    colors: {
      spade: '#3A3A3C',
      heart: '#D63447',
      diamond: '#D4AF37',
      club: '#2E5FA3'
    }
  }
};

export const DEFAULT_DECK_VARIANT: DeckVariantId = 'four-color';

export const PREMIUM_DECK_IDS = new Set<DeckVariantId>([
  'casino', 'bicycle', 'vintage', 'golden', 'pink', 'alt'
]);
