import type {Metadata} from 'next';
import {SystemState} from '@/components/SystemState';

export const metadata: Metadata = {
  title: 'Serviço temporariamente indisponível',
  description: 'O CTech Poker está em manutenção e voltará em breve.',
  robots: {index: false, follow: false}
};

export default function UnavailablePage() {
  return <SystemState
    code="503"
    title="A mesa volta em breve."
    description="Estamos realizando uma manutenção para manter as partidas rápidas, estáveis e seguras."
    detail="Aguarde alguns minutos antes de tentar novamente."
  />;
}
