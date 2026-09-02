import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Reações, cosméticos e fichas',
  description: 'Reações premium, baralhos, feltros, recompensa diária, pacotes via Pix e o histórico de tudo o que você liberou.',
  path: '/guide/store',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
