'use client';
import Link from 'next/link';
import {useId, useState} from 'react';
import {ChevronLeft} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import axios from 'axios';
import {Button} from '@/components/ui/button';
import {getRoom, joinRoom} from '@/lib/api/rooms';
import {isNotFound} from '@/lib/api/client';

const GENERIC_JOIN_ERROR = 'Não foi possível sentar na mesa. Verifique suas fichas e tente novamente.';
const TABLE_FULL_TYPE = '/problems/table-full';
const TABLE_FULL_ERROR = 'A última vaga foi ocupada. Se houve débito, suas fichas já foram devolvidas.';

// The API's real-money buy-in gate (buyin.walletFor) reports "has not
// activated gambling on ctech-wallet" as a plain RFC 9457 `detail` string.
// Surface it verbatim so a real-money player knows to activate their wallet,
// instead of the generic sandbox message.
function joinErrorMessage(err: unknown) {
  if (axios.isAxiosError(err)) {
    const detail = (err.response?.data as {detail?: string} | undefined)?.detail;
    const type = (err.response?.data as {type?: string} | undefined)?.type;
    if (type === TABLE_FULL_TYPE) return TABLE_FULL_ERROR;
    if (detail?.includes('gambling')) return 'Sua carteira ainda não tem apostas ativadas. Ative em ctech-wallet e tente novamente.';
  }
  return GENERIC_JOIN_ERROR;
}

export function midBuyIn(min: number, max: number, bigBlind: number) {
  const bb = bigBlind > 0 ? bigBlind : 1;
  const mid = Math.round((min + max) / 2 / bb) * bb;
  return Math.min(max, Math.max(min, mid));
}

export function formatBuyIn(amount: number, isReal: boolean) {
  return isReal ? (amount / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'}) :
    amount.toLocaleString('pt-BR');
}

/** Buy-in ceremony: the explicit consent step between the lobby and the seat.
 * Nothing is debited until the player confirms an amount. */
export function BuyInPanel({roomId, shareCode, onSeatedAction}: {
  roomId: string;
  shareCode?: string;
  onSeatedAction: () => void
}) {
  const sliderId = useId();
  const queryClient = useQueryClient();
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
  const isReal = room.currency_mode === 'real';
  const unit = isReal ? 'reais' : 'fichas';
  const fmt = (n: number) => formatBuyIn(n, isReal);

  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value, shareCode);
      onSeatedAction();
    } catch (err) {
      setError(joinErrorMessage(err));
      if (axios.isAxiosError(err) &&
        (err.response?.data as {type?: string} | undefined)?.type === TABLE_FULL_TYPE) {
        await Promise.all([
          queryClient.invalidateQueries({queryKey: ['rooms']}),
          queryClient.invalidateQueries({queryKey: ['room', roomId]}),
          queryClient.invalidateQueries({queryKey: ['seated', roomId]}),
        ]);
      }
      setJoining(false);
    }
  }

  return (
    <main className="game-loading buyin">
      <h1 className="sr-only">Mesa de poker</h1>
      <small>BLINDS {room.small_blind} / {room.big_blind} · {room.currency_mode === 'real' ? 'DINHEIRO REAL' : 'SANDBOX'}</small>
      <h2>Sente-se à mesa</h2>
      <p>Escolha {isReal ? 'quanto dinheiro' : 'quantas fichas'} levar. Nada é debitado antes de você confirmar.</p>
      {isReal && !!room.entry_fee_cents &&
        <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
          junto com o buy-in, não é uma comissão sobre o pote).</p>}
      <div className="buyin-control">
        <label htmlFor={sliderId}>Buy-in</label>
        <input id={sliderId} type="range" min={room.buy_in_min} max={room.buy_in_max} step={step} value={value}
               disabled={joining} onChange={event => setAmount(Number(event.target.value))}
               aria-valuetext={`${fmt(value)} ${unit}`}/>
        <output htmlFor={sliderId}>{fmt(value)} <span>{unit}</span></output>
        <small>mín. {fmt(room.buy_in_min)} · máx. {fmt(room.buy_in_max)}</small>
      </div>
      {error && <p className="buyin-error" role="alert">{error}</p>}
      <Button size="lg" onClick={confirm} disabled={joining}>
        {joining ? 'Entrando…' : `Entrar com ${fmt(value)}`}
      </Button>
      <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
    </main>
  );
}
