import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Mesa de poker',
  description: 'Jogue Texas Hold’em em uma mesa viva, responsiva e auditável.',
  path: '/table',
  image: 'table'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
