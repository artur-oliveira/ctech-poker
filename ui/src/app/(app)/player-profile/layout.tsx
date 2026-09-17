import type {Metadata} from 'next';
import type React from 'react';
import {routeMetadata} from '@/lib/routeMetadata';

export const metadata: Metadata = routeMetadata({
  title: 'Seu perfil',
  description: 'Edite nome, foto, baralho e a vitrine pública que outros jogadores veem.',
  path: '/player-profile',
  image: 'profile'
});

export default function Layout({children}: {children: React.ReactNode}) {
  return children;
}
