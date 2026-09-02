import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Mãos, replay e Provably Fair',
  description: 'Encontre uma rodada, reconstrua cada decisão e confira no próprio navegador se as cartas vieram do baralho comprometido.',
  path: '/guide/hands',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
