import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Lobby',
  description: 'Escolha uma mesa sandbox e entre no jogo com seus amigos.',
  path: '/lobby',
  image: 'lobby'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
