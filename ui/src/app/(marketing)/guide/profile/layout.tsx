import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Perfil, vitrine e estatísticas',
  description: 'Como você aparece na mesa, o que fica público e o que as suas mãos dizem sobre o seu jogo.',
  path: '/guide/profile',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
