'use client'

import {zodResolver} from '@hookform/resolvers/zod'
import {useQuery, useQueryClient} from '@tanstack/react-query'
import {Minus, Plus} from 'lucide-react'
import {useState} from 'react'
import {Controller, useForm, useWatch} from 'react-hook-form'
import {z} from 'zod'
import {Button} from '@/components/ui/button'
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger} from '@/components/ui/dialog'
import {Label} from '@/components/ui/label'
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select'
import {createRoom, listStakes} from '@/lib/api/rooms'

const schema = z.object({
  visibility: z.enum(['public', 'private']),
  stakeIndex: z.number().int().min(0),
  seats: z.number().int().min(2, 'Escolha pelo menos 2 lugares').max(9, 'O máximo é de 9 lugares'),
})
type Values = z.infer<typeof schema>

export function CreateRoomDialog() {
  const [open, setOpen] = useState(false)
  const {data: stakes = []} = useQuery({queryKey: ['stakes'], queryFn: listStakes})
  const queryClient = useQueryClient()
  const form = useForm<Values>({resolver: zodResolver(schema), defaultValues: {visibility: 'public', stakeIndex: 0, seats: 6}})
  const seats = useWatch({control: form.control, name: 'seats'})

  async function submit(values: Values) {
    const stake = stakes[values.stakeIndex]
    if (!stake) {
      form.setError('stakeIndex', {message: 'Selecione um stake disponível'})
      return
    }
    try {
      await createRoom({visibility: values.visibility, small_blind: stake.small_blind, big_blind: stake.big_blind, max_seats: values.seats, buy_in_min: stake.big_blind * 20, buy_in_max: stake.big_blind * 100})
      await queryClient.invalidateQueries({queryKey: ['rooms']})
      setOpen(false)
      form.reset()
    } catch {
      form.setError('root', {message: 'Não foi possível criar a mesa. Tente novamente.'})
    }
  }

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button size="lg"/>}><Plus/>Criar mesa</DialogTrigger>
    <DialogContent>
      <DialogHeader><p className="font-mono text-xs tracking-widest text-[var(--brand-bright)]">NOVA MESA</p><DialogTitle>Configure sua mesa</DialogTitle><DialogDescription>Escolha como quer jogar. Os valores abaixo são fichas virtuais do sandbox.</DialogDescription></DialogHeader>
      <form onSubmit={form.handleSubmit(submit)} className="space-y-5">
        <div className="space-y-2"><Label id="visibility-label">Tipo de mesa</Label><Controller control={form.control} name="visibility" render={({field}) => <Select value={field.value} onValueChange={field.onChange}><SelectTrigger aria-labelledby="visibility-label"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="public">Pública</SelectItem><SelectItem value="private">Privada</SelectItem></SelectContent></Select>}/>{form.formState.errors.visibility && <p className="form-error">{form.formState.errors.visibility.message}</p>}</div>
        <div className="space-y-2"><Label id="stake-label">Stakes sandbox</Label><Controller control={form.control} name="stakeIndex" render={({field}) => <Select value={field.value} onValueChange={field.onChange} disabled={!stakes.length}><SelectTrigger aria-labelledby="stake-label"><SelectValue>{stakes[field.value] ? `${stakes[field.value].small_blind} / ${stakes[field.value].big_blind}` : 'Nenhum stake disponível'}</SelectValue></SelectTrigger><SelectContent>{stakes.map((stake, index) => <SelectItem value={index} key={`${stake.small_blind}-${stake.big_blind}`}>{stake.small_blind} / {stake.big_blind}</SelectItem>)}</SelectContent></Select>}/>{form.formState.errors.stakeIndex && <p className="form-error">{form.formState.errors.stakeIndex.message}</p>}</div>
        <div className="space-y-2"><Label id="seats-label">Lugares</Label><div className="flex items-center justify-between rounded-xl border border-white/15 bg-[var(--surface-control)] p-1" role="group" aria-labelledby="seats-label"><Button type="button" variant="ghost" size="icon" aria-label="Diminuir quantidade de lugares" disabled={seats <= 2} onClick={() => form.setValue('seats', Math.max(2, seats - 1), {shouldDirty: true, shouldValidate: true})}><Minus/></Button><output className="min-w-24 text-center" aria-live="polite"><b className="text-lg">{seats}</b><span className="ml-2 text-sm text-[var(--muted-rose)]">lugares</span></output><Button type="button" variant="ghost" size="icon" aria-label="Aumentar quantidade de lugares" disabled={seats >= 9} onClick={() => form.setValue('seats', Math.min(9, seats + 1), {shouldDirty: true, shouldValidate: true})}><Plus/></Button></div>{form.formState.errors.seats && <p className="form-error">{form.formState.errors.seats.message}</p>}</div>
        {form.formState.errors.root && <p className="form-error">{form.formState.errors.root.message}</p>}
        <DialogFooter><Button type="submit" size="lg" disabled={form.formState.isSubmitting || !stakes.length}>{form.formState.isSubmitting ? 'Criando…' : 'Criar mesa'}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
}
