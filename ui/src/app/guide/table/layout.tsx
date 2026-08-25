import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Tudo o que acontece na mesa',
  description: 'Controles de aposta, tempo de decisão, ferramentas sociais, resultados e situações especiais em uma única referência.',
  path: '/guide/table',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
