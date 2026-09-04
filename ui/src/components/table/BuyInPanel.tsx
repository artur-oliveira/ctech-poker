'use client';
import Link from 'next/link';
import {useId, useRef, useState} from 'react';
import {useRouter} from 'next/navigation';
import {ChevronLeft, RefreshCw} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import axios from 'axios';
import {Button} from '@/components/ui/button';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {getRoom, joinOrCreateRoom, joinRoom, type Room} from '@/lib/api/rooms';
import {isNotFound} from '@/lib/api/client';
import {pushNotification} from '@/lib/notify';
import {type LobbyBucket, ROOM_BUCKETS_QUERY_KEY, tableBucketHref} from '@/lib/lobbyBuckets';
import {buyInRange} from '@/lib/pokerRules';

const GENERIC_JOIN_ERROR = 'Não foi possível sentar na mesa. Verifique suas fichas e tente novamente.';
const TABLE_FULL_TYPE = '/problems/table-full';
const TABLE_FULL_ERROR = 'A última vaga foi ocupada. Se houve débito, suas fichas já foram devolvidas.';
// A seat can still fill between opening a table by id and confirming the
// buy-in. Rather than strand the player on a dead table page, re-enter the
// same bucket: the next confirm goes through join-or-create, which resolves
// a free table (or opens one) for the same blinds/format server-side.
const TABLE_FULL_RETRY_NOTICE = `${TABLE_FULL_ERROR} Tentando outra mesa nesses blinds.`;

function isTableFullError(err: unknown) {
  return axios.isAxiosError(err) &&
    (err.response?.data as { type?: string } | undefined)?.type === TABLE_FULL_TYPE;
}

// The API's real-money buy-in gate (buyin.walletFor) reports "has not
// activated gambling on ctech-wallet" as a plain RFC 9457 `detail` string.
// Surface it verbatim so a real-money player knows to activate their wallet,
// instead of the generic sandbox message.
function joinErrorMessage(err: unknown) {
  if (axios.isAxiosError(err)) {
    const detail = (err.response?.data as { detail?: string } | undefined)?.detail;
    if (isTableFullError(err)) return TABLE_FULL_ERROR;
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

// A bucket entry has no room yet — join-or-create picks one on confirm — so
// the ceremony is rendered from the pick itself. Blinds and seats come from
// the URL; the buy-in window is the shared client rule the server enforces
// for public tables anyway (lib/pokerRules).
function roomFromBucket(bucket: LobbyBucket): Room {
  const {min, max} = buyInRange(bucket.bigBlind);
  return {
    visibility: 'public', currency_mode: 'sandbox', status: 'waiting', seats_taken: 0,
    small_blind: bucket.smallBlind, big_blind: bucket.bigBlind, max_seats: bucket.maxSeats,
    buy_in_min: min, buy_in_max: max,
  };
}

/** Buy-in ceremony: the explicit consent step between the lobby and the seat.
 * Nothing is debited until the player confirms an amount.
 *
 * Two entries: a known table (`roomId` — a direct link, an invite, a return to
 * a table) buys in with `POST /rooms/:id/join`; a lobby pick (`bucket`) never
 * names a room and confirms with `POST /rooms/join-or-create`, which resolves
 * the table server-side in the same round trip (#205). */
export function BuyInPanel({roomId = '', bucket, shareCode, onSeatedAction}: {
  roomId?: string;
  bucket?: LobbyBucket;
  shareCode?: string;
  onSeatedAction: (roomId: string) => void
}) {
  const sliderId = useId();
  const autoRebuyId = useId();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [amount, setAmount] = useState<number | null>(null);
  const [autoRebuy, setAutoRebuy] = useState(false);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState('');
  // A bucket entry regenerates the key only when the player moves the slider:
  // retrying the same confirmed amount must reuse it (so a network retry
  // re-seats at the same table instead of buying a second seat), while a
  // deliberately different amount is a different buy-in.
  const idemRef = useRef<{ amount: number; key: string } | null>(null);
  function idemKeyFor(amount: number) {
    if (idemRef.current?.amount !== amount) idemRef.current = {amount, key: crypto.randomUUID()};
    return idemRef.current.key;
  }
  const {data: fetchedRoom, isLoading, error: roomError, isError, refetch} = useQuery({
    queryKey: ['room', roomId],
    queryFn: () => getRoom(roomId),
    enabled: !bucket
  });
  const room = bucket ? roomFromBucket(bucket) : fetchedRoom;

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
    if (!room) return;
    setJoining(true);
    setError('');
    try {
      if (bucket) {
        // One mutation for the whole entry: the server picks or opens the
        // table inside this bucket and seats the player, so losing the last
        // seat to a concurrent joiner resolves into another table here,
        // without a bounce back to the lobby.
        const {room_id} = await joinOrCreateRoom({
          small_blind: bucket.smallBlind, big_blind: bucket.bigBlind, max_seats: bucket.maxSeats,
          amount: value, auto_rebuy: autoRebuy || undefined, idem_key: idemKeyFor(value),
        });
        await queryClient.invalidateQueries({queryKey: ROOM_BUCKETS_QUERY_KEY});
        onSeatedAction(room_id);
        return;
      }
      if (autoRebuy) {
        await joinRoom(roomId, value, shareCode, true);
      } else {
        await joinRoom(roomId, value, shareCode);
      }
      onSeatedAction(roomId);
    } catch (err) {
      if (isTableFullError(err)) {
        await Promise.all([
          queryClient.invalidateQueries({queryKey: ROOM_BUCKETS_QUERY_KEY}),
          queryClient.invalidateQueries({queryKey: ['room', roomId]}),
          queryClient.invalidateQueries({queryKey: ['seated', roomId]}),
        ]);
        // This table filled up between opening it and confirming. Re-enter
        // the same bucket instead of stranding the player here: join-or-create
        // resolves a free table (or opens one) server-side, so this is a
        // re-entry into the ceremony, not a trip back to the lobby to re-pick.
        pushNotification(TABLE_FULL_RETRY_NOTICE, 'info');
        router.push(tableBucketHref({
          smallBlind: room.small_blind, bigBlind: room.big_blind, maxSeats: room.max_seats,
        }));
        return;
      }
      setError(joinErrorMessage(err));
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
      {!isReal &&
          <div className="buyin-control table-preference-toggle">
              <span><RefreshCw aria-hidden="true"/><span>
            <Label id={`${autoRebuyId}-label`} htmlFor={autoRebuyId}>Auto rebuy</Label>
          <small>Se suas fichas acabarem, compramos automaticamente o mesmo valor para você continuar jogando sem esperar.</small>
        </span>
        </span>
              <Switch id={autoRebuyId} aria-labelledby={`${autoRebuyId}-label`} checked={autoRebuy} disabled={joining}
                      onCheckedChange={setAutoRebuy}/>
          </div>}
      {error && <p className="buyin-error" role="alert">{error}</p>}
      <Button size="lg" onClick={confirm} disabled={joining}>
        {joining ? 'Entrando…' : `Entrar com ${fmt(value)}`}
      </Button>
      <Button variant="ghost" render={<Link href="/lobby"/>}><ChevronLeft/> Voltar ao lobby</Button>
    </main>
  );
}
