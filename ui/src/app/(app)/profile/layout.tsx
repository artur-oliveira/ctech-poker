import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Vitrine do jogador',
  description: 'Conheça as conquistas, o estilo de jogo e as melhores mãos de um jogador.',
  path: '/profile',
  image: 'profile',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
