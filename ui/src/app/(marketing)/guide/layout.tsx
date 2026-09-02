import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Como jogar',
  description: 'Aprenda a criar uma mesa, entrar e jogar sua primeira mão de Texas Hold’em.',
  path: '/guide',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
