'use client';
import {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {getBotWaitStatus, startBotsNow} from '@/lib/api/rooms';
import {Button} from '@/components/ui/button';
import {useLiveNow} from '@/lib/hooks/useLiveNow';

export function BotWaitControls({roomId, connected, expected}: {
  roomId: string; connected: boolean; expected: boolean
}) {
  const now = useLiveNow(true, 1000);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const status = useQuery({
    queryKey: ['bot-wait', roomId], queryFn: () => getBotWaitStatus(roomId),
    refetchInterval: query => query.state.status === 'error' || query.state.data?.enabled === false ||
      query.state.data?.has_bot || query.state.data?.reserved ? false : 5000,
    staleTime: 1000,
  });
  const data = status.data;
  if (!data && expected && status.isError) return <div className="bot-wait-controls">
    <p role="alert">Não foi possível confirmar a espera dos bots.</p>
    <Button type="button" size="sm" variant="outline" disabled={!connected}
            onClick={() => status.refetch()}>Atualizar espera</Button>
  </div>;
  if (!data?.enabled || data.has_bot || data.reserved) return null;

  const seconds = Math.max(0, Math.ceil((data.activate_at - now) / 1000));
  const retry = now - data.activate_at > 10_000;
  async function start() {
    setPending(true);
    setError('');
    try {
      await startBotsNow(roomId);
      await status.refetch();
    } catch {
      setError('Não foi possível preparar os bots. Você pode tentar novamente.');
    } finally {
      setPending(false);
    }
  }

  return <div className="bot-wait-controls">
    <p role="status">{retry ? 'Os bots ainda não entraram.' :
      seconds > 0 ? `Buscando pessoas · bots disponíveis em ${seconds} s` : 'Preparando adversários…'}</p>
    <Button type="button" size="sm" variant="outline" disabled={!connected || pending}
            onClick={start}>{pending ? 'Preparando…' : retry ? 'Tentar novamente' : 'Começar com bots agora'}</Button>
    {error && <p className="bot-wait-error" role="alert">{error}</p>}
  </div>;
}
