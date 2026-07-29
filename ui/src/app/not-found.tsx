import type {Metadata} from 'next';
import {SystemState} from '@/components/SystemState';

export const metadata: Metadata = {
  title: 'Página não encontrada',
  robots: {index: false, follow: false}
};

export default function NotFound() {
  return <SystemState
    code="404"
    title="Esta mesa não existe."
    description="O endereço pode estar incompleto, ter expirado ou apontar para uma sala que já foi encerrada."
    detail="Confira o link recebido ou escolha outra mesa disponível no lobby."
  />;
}
