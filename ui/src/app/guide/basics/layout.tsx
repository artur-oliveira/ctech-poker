import {routeMetadata} from '@/lib/routeMetadata';
import {Metadata} from "next";
import React from "react";

export const metadata: Metadata = routeMetadata({
  title: 'Do lobby à primeira mão',
  description: 'Como escolher uma mesa, quantas fichas levar e quando a próxima mão começa.',
  path: '/guide/basics',
  image: 'guide',
  index: true
});
export default function Layout({children}: { children: React.ReactNode }) {
  return children;
}
