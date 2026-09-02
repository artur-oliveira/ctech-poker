import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Como funcionam as conquistas',
  description: 'Cada meta acumula progresso a partir das mãos concluídas e libera estrelas em marcos sucessivos.',
  path: '/guide/achievements',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
