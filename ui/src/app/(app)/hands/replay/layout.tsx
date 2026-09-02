import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Replay da mão',
  description: 'Reviva stacks, pot e decisões na ordem em que aconteceram.',
  path: '/hands/replay',
  image: 'hand-replay'
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
