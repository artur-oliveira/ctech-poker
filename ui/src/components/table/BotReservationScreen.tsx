'use client';

import Link from 'next/link';
import {useEffect, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {useRouter} from 'next/navigation';
import {ChevronLeft, Users} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {cancelBotReservation, getBotReservation, getRoom} from '@/lib/api/rooms';
import {chipsExact} from '@/lib/chips';

export function BotReservationScreen({roomId, reservationId}: { roomId: string; reservationId: string }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [cancelling, setCancelling] = useState(false);
  const reservation = useQuery({
    queryKey: ['bot-reservation', roomId, reservationId],
    queryFn: () => getBotReservation(roomId, reservationId),
    refetchInterval: query => {
      const status = query.state.data?.status;
      if (status === 'expired' || status === 'failed' || status === 'seated') return false;
      return query.state.dataUpdateCount + query.state.fetchFailureCount < 10 ? 2_000 : 5_000;
    },
  });
  const room = useQuery({queryKey: ['room', roomId], queryFn: () => getRoom(roomId)});
  useEffect(() => {
    if (reservation.data?.status !== 'seated') return;
    queryClient.setQueryData(['seated', roomId], {seated: true, stack: 0});
    router.replace(`/table?id=${encodeURIComponent(roomId)}`);
  }, [queryClient, reservation.data?.status, roomId, router]);
  const expired = reservation.data?.status === 'expired';
  const failed = reservation.data?.status === 'failed';
  async function cancel() {
    setCancelling(true);
    try {
      await cancelBotReservation(roomId, reservationId);
      router.replace('/lobby');
    } catch {
      void reservation.refetch();
    } finally {
      setCancelling(false);
    }
  }
  return <main className="game-loading buyin">
    <h1 className="sr-only">Reserva de mesa</h1>
    <Users aria-hidden="true"/>
    <h2>{expired ? 'A reserva expirou' : failed ? 'A entrada não foi concluída' : 'Sua vaga está reservada'}</h2>
    {room.data?.small_blind != null && <p><strong>Blinds {chipsExact(room.data.small_blind)} / {chipsExact(room.data.big_blind)}</strong>
      {' · '}{room.data.max_seats === 2 ? 'Heads-up' : `${room.data.max_seats}-max`}
      {reservation.data?.amount ? ` · buy-in ${chipsExact(reservation.data.amount)}` : ''}</p>}
    <p role="status">{expired
      ? 'A mão demorou mais que o período da reserva. Nenhuma ficha foi debitada.'
      : failed ? reservation.data?.reason || 'Não foi possível concluir o buy-in.'
        : 'Uma mão está terminando. Você entra antes da próxima distribuição e suas fichas ainda não foram debitadas.'}</p>
    {!expired && !failed && !reservation.isError && <small>Conectado · acompanhando sua reserva</small>}
    {reservation.isError && <p className="buyin-error" role="alert">Não foi possível consultar a reserva. Tentaremos novamente.
      <Button variant="ghost" onClick={() => void reservation.refetch()}>Atualizar agora</Button></p>}
    {!expired && !failed && <Button variant="outline" disabled={cancelling} onClick={() => void cancel()}>
      {cancelling ? 'Cancelando…' : 'Cancelar entrada'}
    </Button>}
    {(expired || failed) && <Button render={<Link href="/lobby"/>}>Tentar outra mesa</Button>}
    <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
  </main>;
}
