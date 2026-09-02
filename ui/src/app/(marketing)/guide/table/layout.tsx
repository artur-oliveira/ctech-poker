import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Tudo o que acontece na mesa',
  description: 'Controles, atalhos, tempo de decisão, cartas, resultados e ferramentas da mesa em uma referência só.',
  path: '/guide/table',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
