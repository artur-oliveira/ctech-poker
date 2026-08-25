import {routeMetadata} from '@/lib/routeMetadata';

export const metadata = routeMetadata({
  title: 'Perfil, vitrine e estatísticas',
  description: 'Personalize como você aparece na mesa, escolha o que fica público e acompanhe tendências privadas do seu jogo.',
  path: '/guide/profile',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
