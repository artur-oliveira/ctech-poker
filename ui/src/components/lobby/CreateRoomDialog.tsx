'use client';

import {zodResolver} from '@hookform/resolvers/zod';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Lock} from 'lucide-react';
import {useRouter} from 'next/navigation';
import {useState} from 'react';
import {Controller, useForm} from 'react-hook-form';
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
import {createRoom, listStakes} from '@/lib/api/rooms';

const MAX_SEATS_OPTIONS = [6, 9] as const;

const schema = z.object({
  stakeIndex: z.number().int().min(0),
  maxSeats: z.union([z.literal(6), z.literal(9)]),
});
type Values = z.infer<typeof schema>

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
  const {data: stakes = []} = useQuery({queryKey: ['stakes'], queryFn: listStakes});
  const queryClient = useQueryClient();
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {stakeIndex: 0, maxSeats: 6}
  });

  async function submit(values: Values) {
    const stake = stakes[values.stakeIndex];
    if (!stake) {
      form.setError('stakeIndex', {message: 'Selecione um stake disponível'});
      return;
    }
    try {
      const room = await createRoom({
        visibility: 'private',
        small_blind: stake.small_blind,
        big_blind: stake.big_blind,
        max_seats: values.maxSeats,
        buy_in_min: stake.big_blind * 40,
        buy_in_max: stake.big_blind * 100
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
        sua mesa</DialogTitle><DialogDescription>Convide amigos por link. Os valores abaixo são fichas virtuais do
        sandbox.</DialogDescription></DialogHeader>
      <form onSubmit={form.handleSubmit(submit)} className="space-y-5">
        <div className="space-y-2"><Label id="stake-label">Stakes sandbox</Label><Controller control={form.control}
          name="stakeIndex"
          render={({field}) => <div className="flex flex-wrap gap-2" role="radiogroup" aria-labelledby="stake-label">
            {stakes.map((stake, index) => <button type="button" key={`${stake.small_blind}-${stake.big_blind}`}
              role="radio" aria-checked={field.value === index} tabIndex={field.value === index ? 0 : -1}
              className={`rounded-xl border px-4 py-2 min-h-11 text-sm font-semibold transition-colors ${field.value === index ? 'border-[var(--brand-bright)] bg-[var(--brand)] text-[var(--on-brand)]' : 'border-white/15 bg-(--surface-control) text-[var(--on-brand)] hover:bg-white/10'}`}
              onClick={() => field.onChange(index)}
              onKeyDown={e => radioGroupKeyDown(e, index, stakes.length, field.onChange)}>{stake.small_blind.toLocaleString('pt-BR')} / {stake.big_blind.toLocaleString('pt-BR')}</button>)}
          </div>}/>{!stakes.length && <p className="form-error">Nenhum stake disponível no momento.</p>}
          {form.formState.errors.stakeIndex &&
            <p className="form-error">{form.formState.errors.stakeIndex.message}</p>}</div>
        <div className="space-y-2"><Label id="seats-label">Lugares</Label><Controller control={form.control}
          name="maxSeats"
          render={({field}) => <div className="flex flex-wrap gap-2" role="radiogroup" aria-labelledby="seats-label">
            {MAX_SEATS_OPTIONS.map((option, index) => <button type="button" key={option} role="radio"
              aria-checked={field.value === option} tabIndex={field.value === option ? 0 : -1}
              className={`rounded-xl border px-4 py-2 min-h-11 text-sm font-semibold transition-colors ${field.value === option ? 'border-[var(--brand-bright)] bg-[var(--brand)] text-[var(--on-brand)]' : 'border-white/15 bg-(--surface-control) text-[var(--on-brand)] hover:bg-white/10'}`}
              onClick={() => field.onChange(option)}
              onKeyDown={e => radioGroupKeyDown(e, index, MAX_SEATS_OPTIONS.length, next => field.onChange(MAX_SEATS_OPTIONS[next]))}>{option} lugares</button>)}
          </div>}/></div>
        {form.formState.errors.root && <p className="form-error">{form.formState.errors.root.message}</p>}
        <DialogFooter><Button type="submit" size="lg"
          disabled={form.formState.isSubmitting || !stakes.length}>{form.formState.isSubmitting ? 'Criando…' : 'Criar mesa privada'}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
