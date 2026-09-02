import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";

export const metadata: Metadata = routeMetadata({
  title: 'Mão compartilhada',
  description: 'Veja uma mão anonimizada e reproduza as ações na mesa do CTech Poker.',
  path: '/share',
  image: 'shared-hand'
});

export default function ShareLayout({children}: { children: React.ReactNode }) {
  return children;
}
