'use client';
import {useRef, useState} from 'react';
import Image from 'next/image';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Activity, Camera, LoaderCircle, LogOut, Trash2} from 'lucide-react';
import {getMe, updateMe, type WalletMode} from '@/lib/api/player';
import {logout} from '@/lib/auth/oauth';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId, DEFAULT_DECK_VARIANT} from '@/lib/cardVariants';
import {ProfileShowcaseDialog} from '@/components/lobby/ProfileShowcaseDialog';
import {SelfHudDialog} from '@/components/lobby/SelfHudDialog';
import {deleteAvatar, uploadAvatar} from '@/lib/avatar';
import {pushNotification} from '@/lib/notify';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

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
  const [showcaseOpen, setShowcaseOpen] = useState(false);
  const [selfHudOpen, setSelfHudOpen] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: data => {
      queryClient.setQueryData(['player', 'me'], data);
      setEditingName(false);
    }
  });
  const avatar = useMutation({
    mutationFn: uploadAvatar,
    onSuccess: data => {
      queryClient.setQueryData(['player', 'me'], data);
      pushNotification('Foto de perfil atualizada.', 'info');
    },
    onError: () => pushNotification('Não foi possível atualizar a foto. Tente outra imagem.'),
  });
  const removeAvatar = useMutation({
    mutationFn: deleteAvatar,
    onSuccess: data => {
      queryClient.setQueryData(['player', 'me'], data);
      pushNotification('Foto de perfil removida.', 'info');
    },
  });

  const walletMode: WalletMode = me?.wallet_mode || 'sandbox';
  const deckVariant: DeckVariantId = me?.deck_variant || DEFAULT_DECK_VARIANT;
  const balanceLabel = walletMode === 'real' ? formatReal(me?.game_balance) : formatSandbox(me?.sandbox_balance);

  return <><Popover onOpenChange={(open, details) => {
    if (!open && editingName && details.reason === 'escape-key') {
      details.cancel();
      setEditingName(false);
    }
  }}>
    <div className="profile-summary">
      <span className="balance-pill">{balanceLabel}</span>
      <PopoverTrigger render={<Button variant="ghost" size="icon" className="rounded-full" aria-label="Abrir perfil"/>}>
        <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url}/>
      </PopoverTrigger>
    </div>
    <PopoverContent>
      <div className="space-y-4">
        <div className="profile-avatar-editor">
          <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url} size={56}/>
          <div className="profile-avatar-copy">
            <b>Foto do perfil</b>
            <span>JPG ou PNG · corte quadrado</span>
          </div>
          <input ref={fileInput} className="sr-only" type="file" accept="image/jpeg,image/png"
                 aria-label="Selecionar foto de perfil" onChange={event => {
                   const file = event.target.files?.[0];
                   if (file) avatar.mutate(file);
                   event.target.value = '';
                 }}/>
          <div className="profile-avatar-actions">
            <Button type="button" size="sm" variant="outline" disabled={avatar.isPending}
                    onClick={() => fileInput.current?.click()}>
              {avatar.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Camera aria-hidden="true"/>}
              {avatar.isPending ? 'Enviando…' : me?.avatar_url ? 'Trocar' : 'Adicionar'}
            </Button>
            {me?.avatar_url && <Button type="button" size="icon" variant="ghost" disabled={removeAvatar.isPending}
                                       aria-label="Remover foto de perfil" onClick={() => removeAvatar.mutate()}>
              <Trash2 aria-hidden="true"/>
            </Button>}
          </div>
        </div>
        <div className="space-y-2">
          <Label id="profile-name-label">Nome de exibição</Label>
          {editingName ? (
            <div className="flex gap-2">
              <Input aria-labelledby="profile-name-label" value={name} onChange={e => setName(e.target.value)}
                     autoFocus onKeyDown={e => {
                if (e.key === 'Enter' && name.trim()) save.mutate({name: name.trim()});
                if (e.key === 'Escape') setEditingName(false);
              }}/>
              <Button size="sm" disabled={!name.trim() || save.isPending}
                      onClick={() => save.mutate({name: name.trim()})}>
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
          <Switch aria-labelledby="wallet-mode-label" checked={walletMode === 'real'}
                  onCheckedChange={checked => save.mutate({wallet_mode: checked ? 'real' : 'sandbox'})}/>
        </div>
        <div className="space-y-2">
          <Label id="deck-variant-label">Baralho</Label>
          <Select value={deckVariant}
                  onValueChange={(value: DeckVariantId | null) => value && save.mutate({deck_variant: value})}>
            <SelectTrigger aria-labelledby="deck-variant-label">
              <SelectValue>
                {(value: DeckVariantId) => DECK_VARIANTS[value]?.label ?? DECK_VARIANTS[DEFAULT_DECK_VARIANT].label}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {Object.entries(DECK_VARIANTS).map(([id, variant]) => (
                <SelectItem key={id} value={id as DeckVariantId} label={variant.label}>
                  <span className="deck-variant-option">
                    <span className="deck-variant-option-cards">
                      {ACES.map(card => <Image
                        key={card} src={cardPath(card, id as DeckVariantId)} alt=""
                        height={0}
                        width={0}
                        style={{width: '20px', height: "auto"}}/>
                      )}
                    </span>
                    {variant.label}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="profile-balances">
          <span>Fichas sandbox <b>{formatSandbox(me?.sandbox_balance)}</b></span>
          <span>Dinheiro real <b>{formatReal(me?.game_balance)}</b></span>
        </div>
        <Button type="button" variant="outline" className="w-full" onClick={() => setShowcaseOpen(true)}>
          Vitrine do perfil
        </Button>
        <Button type="button" variant="outline" className="w-full" onClick={() => setSelfHudOpen(true)}>
          <Activity/> Seu jogo
        </Button>
        <Button variant="outline" className="w-full" onClick={() => logout()}><LogOut/> Sair da conta</Button>
      </div>
    </PopoverContent>
  </Popover>
    <ProfileShowcaseDialog open={showcaseOpen} onOpenChange={setShowcaseOpen}/>
    <SelfHudDialog open={selfHudOpen} onOpenChange={setSelfHudOpen}/>
  </>;
}
