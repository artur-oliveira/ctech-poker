'use client';
import {useState} from 'react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {LogOut} from 'lucide-react';
import {getMe, updateMe, type WalletMode} from '@/lib/api/player';
import {logout} from '@/lib/auth/oauth';
import {Avatar, AvatarFallback} from '@/components/ui/avatar';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';

function initials(name?: string) {
  if (!name) return '?';
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] || '') + (parts.length > 1 ? parts[parts.length - 1][0] : '')).toUpperCase() || '?';
}

function formatSandbox(amount?: number) {
  return `${(amount ?? 0).toLocaleString('pt-BR')} fichas`;
}

function formatReal(amount?: number) {
  return `R$ ${(amount ?? 0).toLocaleString('pt-BR', {minimumFractionDigits: 2, maximumFractionDigits: 2})}`;
}

export function ProfileMenu() {
  const queryClient = useQueryClient();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  const [name, setName] = useState('');
  const [editingName, setEditingName] = useState(false);

  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: data => {
      queryClient.setQueryData(['player', 'me'], data);
      setEditingName(false);
    }
  });

  const walletMode: WalletMode = me?.wallet_mode || 'sandbox';
  const balanceLabel = walletMode === 'real' ? formatReal(me?.game_balance) : formatSandbox(me?.sandbox_balance);

  return <Popover>
    <div className="profile-summary">
      <span className="balance-pill">{balanceLabel}</span>
      <PopoverTrigger render={<Button variant="ghost" size="icon" className="rounded-full" aria-label="Abrir perfil"/>}>
        <Avatar><AvatarFallback>{initials(me?.name)}</AvatarFallback></Avatar>
      </PopoverTrigger>
    </div>
    <PopoverContent>
      <div className="space-y-4">
        <div className="space-y-2">
          <Label id="profile-name-label">Nome de exibição</Label>
          {editingName ? (
            <div className="flex gap-2">
              <Input aria-labelledby="profile-name-label" value={name} onChange={e => setName(e.target.value)}
                autoFocus onKeyDown={e => {
                  if (e.key === 'Enter' && name.trim()) save.mutate({name: name.trim()});
                  if (e.key === 'Escape') setEditingName(false);
                }}/>
              <Button size="sm" disabled={!name.trim() || save.isPending} onClick={() => save.mutate({name: name.trim()})}>
                Salvar
              </Button>
            </div>
          ) : (
            <button type="button" className="profile-name-display" onClick={() => {
              setName(me?.name || '');
              setEditingName(true);
            }}>
              {me?.name || 'Definir nome'}
            </button>
          )}
        </div>
        <div className="flex items-center justify-between">
          <Label id="wallet-mode-label">{walletMode === 'real' ? 'Dinheiro real' : 'Sandbox'}</Label>
          <Switch aria-labelledby="wallet-mode-label" checked={walletMode === 'real'} disabled={save.isPending}
            onCheckedChange={checked => save.mutate({wallet_mode: checked ? 'real' : 'sandbox'})}/>
        </div>
        <div className="profile-balances">
          <span>Fichas sandbox <b>{formatSandbox(me?.sandbox_balance)}</b></span>
          <span>Dinheiro real <b>{formatReal(me?.game_balance)}</b></span>
        </div>
        <Button variant="outline" className="w-full" onClick={() => logout()}><LogOut/> Sair da conta</Button>
      </div>
    </PopoverContent>
  </Popover>;
}
