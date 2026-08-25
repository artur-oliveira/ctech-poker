import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Reações, fichas e compras',
  description: 'A Loja reúne reações permanentes, saldo sandbox, recompensa gratuita, pacotes via Pix e o histórico de compras.',
  path: '/guide/store',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
