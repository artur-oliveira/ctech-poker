import type {Metadata} from 'next';

export const metadata: Metadata = {
  title: 'Mão compartilhada',
  description: 'Veja uma mão anonimizada e reproduza as ações na mesa do CTech Poker.',
  openGraph: {
    title: 'Mão compartilhada · CTech Poker',
    description: 'Board, resultado e replay de uma mão compartilhada com privacidade.',
    images: ['/og-image.png']
  },
  robots: {index: false, follow: false}
};

export default function ShareLayout({children}: {children: React.ReactNode}) {
  return children;
}
