import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Detalhes da mão',
  description: 'Revise board, apostas, resultado e prova de embaralhamento.',
  path: '/hands/history',
  image: 'hand-history'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
