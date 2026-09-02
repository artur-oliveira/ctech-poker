import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Ranking, convivência e jogo seguro',
  description: 'Como encontrar gente para jogar, o que os outros veem de você e os controles de tempo, privacidade e conexão.',
  path: '/guide/community',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
