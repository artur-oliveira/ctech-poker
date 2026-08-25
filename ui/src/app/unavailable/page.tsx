import type {Metadata} from 'next';
import {UnavailableState} from './UnavailableState';

export const metadata: Metadata = {
  title: 'Serviço temporariamente indisponível',
  description: 'O CTech Poker está em manutenção e voltará em breve.',
  robots: {index: false, follow: false}
};

export default function UnavailablePage() {
  return <UnavailableState/>;
}
