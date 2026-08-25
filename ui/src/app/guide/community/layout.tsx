import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Ranking, convivência e jogo seguro',
  description: 'Recursos que conectam jogadores, preservam contexto entre sessões e ajudam a manter controle sobre tempo, privacidade e conexão.',
  path: '/guide/community',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
