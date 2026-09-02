import {routeMetadata} from '@/lib/routeMetadata';
import React from "react";
import {Metadata} from "next";

export const metadata: Metadata = routeMetadata({
  title: 'Pessoas',
  description: 'Amigos, solicitações, jogadores recentes e sua atividade social nas mesas.',
  path: '/people',
  image: 'lobby'
});

export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
