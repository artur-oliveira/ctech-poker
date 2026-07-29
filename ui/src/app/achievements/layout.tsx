import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Conquistas',
  description: 'Acompanhe seus marcos e o progresso conquistado nas mesas.',
  path: '/achievements',
  image: 'achievements'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
