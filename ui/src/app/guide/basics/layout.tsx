import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Do lobby à primeira mão',
  description: 'O caminho mais curto para encontrar uma mesa, escolher seu buy-in e entender quando a partida começa.',
  path: '/guide/basics',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
