import type {Metadata} from 'next';
import type {ReactNode} from 'react';
import '../../../src/app/globals.css';
import './studio.css';

export const metadata: Metadata = {
  title: 'Estúdio de imagens sociais · CTech Poker',
  robots: {index: false, follow: false}
};

export default function Layout({children}: {children: ReactNode}) {
  return <html lang="pt-BR"><body>{children}</body></html>;
}
