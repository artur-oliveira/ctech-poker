'use client';
import Link from 'next/link';
import {useId, useState} from 'react';
import {ChevronLeft} from 'lucide-react';
import {useQuery} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {getRoom, joinRoom} from '@/lib/api/rooms';
import {isNotFound} from '@/lib/api/client';

function midBuyIn(min: number, max: number, bigBlind: number) {
  const bb = bigBlind > 0 ? bigBlind : 1;
  const mid = Math.round((min + max) / 2 / bb) * bb;
  return Math.min(max, Math.max(min, mid));
}

/** Buy-in ceremony: the explicit consent step between the lobby and the seat.
 * Nothing is debited until the player confirms an amount. */
export function BuyInPanel({roomId, shareCode, onSeatedAction}: {
  roomId: string;
  shareCode?: string;
  onSeatedAction: () => void
}) {
  const sliderId = useId();
  const [amount, setAmount] = useState<number | null>(null);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState('');
  const {data: room, isLoading, error: roomError, isError, refetch} = useQuery({
    queryKey: ['room', roomId],
    queryFn: () => getRoom(roomId),
    retry: (count, err) => !isNotFound(err) && count < 3
  });

  if (isLoading) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <span className="loader"/>
      <h2>Preparando a mesa…</h2>
    </main>
  );
  if (isError && isNotFound(roomError)) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <h2>Essa sala não está mais disponível</h2>
      <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
    </main>
  );
  if (isError || !room || !room.buy_in_max) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <h2>Não foi possível abrir a mesa</h2>
      <p>Confira sua conexão e tente novamente.</p>
      <Button onClick={() => refetch()}>Tentar novamente</Button>
      <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
    </main>
  );

  const step = room.big_blind > 0 ? room.big_blind : 1;
  const value = amount ?? midBuyIn(room.buy_in_min, room.buy_in_max, room.big_blind);

  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value, shareCode);
      onSeatedAction();
    } catch {
      setError('Não foi possível sentar na mesa. Verifique suas fichas e tente novamente.');
      setJoining(false);
    }
  }

  return (
    <main className="game-loading buyin">
      <h1 className="sr-only">Mesa de poker</h1>
      <small>BLINDS {room.small_blind} / {room.big_blind} · {room.currency_mode === 'real' ? 'DINHEIRO REAL' : 'SANDBOX'}</small>
      <h2>Sente-se à mesa</h2>
      <p>Escolha quantas fichas levar. Nada é debitado antes de você confirmar.</p>
      <div className="buyin-control">
        <label htmlFor={sliderId}>Buy-in</label>
        <input id={sliderId} type="range" min={room.buy_in_min} max={room.buy_in_max} step={step} value={value}
          disabled={joining} onChange={event => setAmount(Number(event.target.value))}
          aria-valuetext={`${value.toLocaleString('pt-BR')} fichas`}/>
        <output htmlFor={sliderId}>{value.toLocaleString('pt-BR')} <span>fichas</span></output>
        <small>mín. {room.buy_in_min.toLocaleString('pt-BR')} · máx. {room.buy_in_max.toLocaleString('pt-BR')}</small>
      </div>
      {error && <p className="buyin-error" role="alert">{error}</p>}
      <Button size="lg" onClick={confirm} disabled={joining}>
        {joining ? 'Entrando…' : `Entrar com ${value.toLocaleString('pt-BR')} fichas`}
      </Button>
      <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
    </main>
  );
}
