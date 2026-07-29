import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Mãos jogadas',
  description: 'Consulte resultados, detalhes e provas de embaralhamento das suas mãos.',
  path: '/hands',
  image: 'hands'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
