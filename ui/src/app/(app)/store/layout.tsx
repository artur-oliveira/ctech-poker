import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Loja',
  description: 'Use fichas sandbox para liberar reações, mesas e itens de perfil.',
  path: '/store',
  image: 'store'
});

export default function Layout({children}: {children: React.ReactNode}) {
  return children;
}
