import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Ranking da comunidade',
  description: 'Compare volume, vitórias e consistência dos jogadores da comunidade.',
  path: '/leaderboard',
  image: 'leaderboard'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
