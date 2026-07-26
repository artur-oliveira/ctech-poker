export type Suit = 'spade' | 'heart' | 'diamond' | 'club';

export type DeckVariantId = (
  'two-color' |
  'four-color' |
  'colorblind' |
  'high-constrast' |
  'casino' |
  'bicycle' |
  'dark' |
  'vintage'
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
    label: 'Alto Contraste',
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
  dark: {
    label: 'Modo Escuro',
    colors: {
      spade: '#8B8B8B',
      heart: '#FF6B6B',
      diamond: '#4DABF7',
      club: '#69DB7C'
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
  }
};

export const DEFAULT_DECK_VARIANT: DeckVariantId = 'four-color';
