import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Regras do Texas Hold’em',
  description: 'Ordem das mãos, rodadas de aposta e desempates em português claro.',
  path: '/poker-rules',
  image: 'poker-rules',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
