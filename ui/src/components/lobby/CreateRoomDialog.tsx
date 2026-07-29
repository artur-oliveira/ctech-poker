'use client';

import {zodResolver} from '@hookform/resolvers/zod';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Lock, Repeat2} from 'lucide-react';
import {useRouter} from 'next/navigation';
import {useState} from 'react';
import {Controller, useForm, useWatch} from 'react-hook-form';
import {z} from 'zod';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {Checkbox} from '@/components/ui/checkbox';
import {getMe} from '@/lib/api/player';
import {createRoom, listStakes} from '@/lib/api/rooms';

const MAX_SEATS_OPTIONS = [6, 9] as const;

const schema = z.object({
  stakeIndex: z.number().int().min(0),
  maxSeats: z.union([z.literal(6), z.literal(9)]),
  currencyMode: z.union([z.literal('sandbox'), z.literal('real')]),
  runItTwiceEnabled: z.boolean(),
});
type Values = z.infer<typeof schema>

/** Sandbox stakes are plain chip counts; real stakes are BRL cents (stakes.go: realPublicStakes). */
function formatStake(value: number, mode: 'sandbox' | 'real') {
  return mode === 'real' ? (value / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'})
    : value.toLocaleString('pt-BR');
}

/** ARIA APG radiogroup pattern: arrow keys move the roving tab stop between options. */
function radioGroupKeyDown(e: React.KeyboardEvent<HTMLButtonElement>, index: number, length: number, select: (index: number) => void) {
  const delta = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
  if (!delta) return;
  e.preventDefault();
  const next = (index + delta + length) % length;
  select(next);
  (e.currentTarget.parentElement?.children[next] as HTMLElement | undefined)?.focus();
}

export function CreateRoomDialog() {
  const [open, setOpen] = useState(false);
  const router = useRouter();
  const {data: sandboxStakes = []} = useQuery({queryKey: ['stakes'], queryFn: () => listStakes()});
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  // Only fetch (and only offer) real-money stakes for players already in real-money
  // wallet mode. 404s when REAL_MONEY_ENABLED is off server-side, hiding the toggle.
  const {data: realStakes = []} = useQuery({
    queryKey: ['stakes', 'real'], queryFn: () => listStakes('real'), retry: false,
    enabled: me?.wallet_mode === 'real'
  });
  const queryClient = useQueryClient();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {stakeIndex: 0, maxSeats: 6, currencyMode: 'sandbox', runItTwiceEnabled: false}
  });
  const currencyMode = useWatch({control: form.control, name: 'currencyMode'});
  const stakes = currencyMode === 'real' ? realStakes : sandboxStakes;
  
  async function submit(values: Values) {
    const stake = stakes[values.stakeIndex];
    if (!stake) {
      form.setError('stakeIndex', {message: 'Selecione um stake disponível'});
      return;
    }
    try {
      const room = await createRoom({
        visibility: 'private',
        currency_mode: values.currencyMode,
        small_blind: stake.small_blind,
        big_blind: stake.big_blind,
        max_seats: values.maxSeats,
        buy_in_min: stake.big_blind * 40,
        buy_in_max: stake.big_blind * 100,
        run_it_twice_enabled: values.runItTwiceEnabled
      });
      await queryClient.invalidateQueries({queryKey: ['rooms']});
      setOpen(false);
      form.reset();
      const roomID = room.room_id || room.id || '';
      if (roomID) router.push(`/table?id=${encodeURIComponent(roomID)}`);
    } catch {
      form.setError('root', {message: 'Não foi possível criar a mesa. Tente novamente.'});
    }
  }
  
  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button size="lg" variant="outline"/>}><Lock/>Mesa privada</DialogTrigger>
    <DialogContent>
      <DialogHeader><p className="font-mono text-xs tracking-widest text-(--brand-bright)">MESA PRIVADA</p><DialogTitle>Configure
        sua mesa</DialogTitle><DialogDescription>Convide amigos por link. {currencyMode === 'real'
        ? 'Os valores abaixo são em dinheiro real, debitados da sua carteira ctech-wallet.'
        : 'Os valores abaixo são fichas virtuais do sandbox.'}</DialogDescription></DialogHeader>
      <form onSubmit={form.handleSubmit(submit)} className="space-y-5">
        {realStakes.length > 0 &&
            <div className="space-y-2"><Label id="currency-label">Modo</Label><Controller control={form.control}
                                                                                          name="currencyMode"
                                                                                          render={({field}) => <div
                                                                                            className="flex flex-wrap gap-2"
                                                                                            role="radiogroup"
                                                                                            aria-labelledby="currency-label">
                                                                                            {(['sandbox', 'real'] as const).map((option, index) =>
                                                                                              <button type="button"
                                                                                                      key={option}
                                                                                                      role="radio"
                                                                                                      aria-checked={field.value === option}
                                                                                                      tabIndex={field.value === option ? 0 : -1}
                                                                                                      className={`rounded-xl border px-4 py-2 min-h-11 text-sm font-semibold transition-colors ${field.value === option ? 'border-[var(--brand-bright)] bg-[var(--brand)] text-[var(--on-brand)]' : 'border-white/15 bg-(--surface-control) text-[var(--on-brand)] hover:bg-white/10'}`}
                                                                                                      onClick={() => {
                                                                                                        field.onChange(option);
                                                                                                        form.setValue('stakeIndex', 0);
                                                                                                      }}
                                                                                                      onKeyDown={e => radioGroupKeyDown(e, index, 2, next => {
                                                                                                        field.onChange((['sandbox', 'real'] as const)[next]);
                                                                                                        form.setValue('stakeIndex', 0);
                                                                                                      })}>{option === 'real' ? 'Dinheiro real' : 'Sandbox'}</button>)}
                                                                                          </div>}/></div>}
        {currencyMode === 'real' &&
            <p className="form-error" role="alert">Dinheiro real exige ativação de apostas na sua carteira
                ctech-wallet. Jogue com responsabilidade.</p>}
        <div className="space-y-2"><Label
          id="stake-label">Stakes {currencyMode === 'real' ? 'em dinheiro real' : 'sandbox'}</Label><Controller
          control={form.control}
          name="stakeIndex"
          render={({field}) => <div
            className="flex flex-wrap gap-2"
            role="radiogroup"
            aria-labelledby="stake-label">
            {stakes.map((stake, index) =>
              <button type="button"
                      key={`${stake.small_blind}-${stake.big_blind}`}
                      role="radio"
                      aria-checked={field.value === index}
                      tabIndex={field.value === index ? 0 : -1}
                      className={`rounded-xl border px-4 py-2 min-h-11 text-sm font-semibold transition-colors ${field.value === index ? 'border-[var(--brand-bright)] bg-[var(--brand)] text-[var(--on-brand)]' : 'border-white/15 bg-(--surface-control) text-[var(--on-brand)] hover:bg-white/10'}`}
                      onClick={() => field.onChange(index)}
                      onKeyDown={e => radioGroupKeyDown(e, index, stakes.length, field.onChange)}>
                {formatStake(stake.small_blind, currencyMode)} / {formatStake(stake.big_blind, currencyMode)}
                {currencyMode === 'real' && stake.fee_cents ? <><br/><small
                  className="opacity-80">taxa {formatStake(stake.fee_cents, 'real')}</small></> : null}
              </button>)}
          </div>}/>{!stakes.length &&
            <p className="form-error">Nenhum stake disponível no momento.</p>}
          {form.formState.errors.stakeIndex &&
              <p className="form-error">{form.formState.errors.stakeIndex.message}</p>}</div>
        <div className="space-y-2"><Label id="seats-label">Lugares</Label><Controller control={form.control}
                                                                                      name="maxSeats"
                                                                                      render={({field}) => <div
                                                                                        className="flex flex-wrap gap-2"
                                                                                        role="radiogroup"
                                                                                        aria-labelledby="seats-label">
                                                                                        {MAX_SEATS_OPTIONS.map((option, index) =>
                                                                                          <button type="button"
                                                                                                  key={option}
                                                                                                  role="radio"
                                                                                                  aria-checked={field.value === option}
                                                                                                  tabIndex={field.value === option ? 0 : -1}
                                                                                                  className={`rounded-xl border px-4 py-2 min-h-11 text-sm font-semibold transition-colors ${field.value === option ? 'border-[var(--brand-bright)] bg-[var(--brand)] text-[var(--on-brand)]' : 'border-white/15 bg-(--surface-control) text-[var(--on-brand)] hover:bg-white/10'}`}
                                                                                                  onClick={() => field.onChange(option)}
                                                                                                  onKeyDown={e => radioGroupKeyDown(e, index, MAX_SEATS_OPTIONS.length, next => field.onChange(MAX_SEATS_OPTIONS[next]))}>{option} lugares</button>)}
                                                                                      </div>}/></div>
        <Controller control={form.control} name="runItTwiceEnabled" render={({field}) =>
          <label className="create-room-option">
            <Checkbox checked={field.value} onCheckedChange={value => field.onChange(value === true)}/>
            <Repeat2 aria-hidden="true"/>
            <span><b>Permitir rodar duas vezes</b>
              <small>Cada jogador decide por si. Em um all-in, todos os envolvidos precisam ter ativado.</small>
            </span>
          </label>}/>
        {form.formState.errors.root && <p className="form-error">{form.formState.errors.root.message}</p>}
        <DialogFooter><Button type="submit" size="lg"
                              disabled={form.formState.isSubmitting || !stakes.length}>{form.formState.isSubmitting ? 'Criando…' : 'Criar mesa privada'}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
