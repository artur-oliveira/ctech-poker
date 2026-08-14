'use client';
import {useRef, useState} from 'react';
import Image from 'next/image';
import Link from 'next/link';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {
  Activity,
  Camera,
  Check,
  ChevronRight,
  Eye,
  LoaderCircle,
  LogOut,
  Pencil,
  ShoppingBag,
  Sparkles,
  Trash2,
  WalletCards,
  X
} from 'lucide-react';
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
    onSuccess: (data, input) => {
      queryClient.setQueryData(['player', 'me'], data);
      setEditingName(false);
      if (input?.name) pushNotification(`Agora você joga como ${data.name}.`, 'info');
      if (input?.deck_variant) pushNotification('Baralho pronto para a próxima mão.', 'info');
      if (input?.wallet_mode) pushNotification(
        input.wallet_mode === 'sandbox' ? 'Modo sandbox selecionado.' : 'Modo dinheiro real selecionado.',
        'info'
      );
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
      <Link href="/store" className="balance-pill" aria-label={`Abrir loja. Saldo: ${balanceLabel}`}>
        {balanceLabel}
      </Link>
      <PopoverTrigger render={<Button variant="ghost" size="icon" className="rounded-full" aria-label="Abrir perfil"/>}>
        <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url}/>
      </PopoverTrigger>
    </div>
    <PopoverContent className="profile-menu-content" aria-label="Perfil e preferências">
      <div className="profile-menu">
        <header className="profile-menu-identity">
          <div className="profile-menu-avatar">
            <PlayerAvatar name={me?.name} avatarUrl={me?.avatar_url} size={64}/>
            <Button type="button" size="icon" className="profile-avatar-camera" disabled={avatar.isPending}
                    aria-label={me?.avatar_url ? 'Trocar foto de perfil' : 'Adicionar foto de perfil'}
                    onClick={() => fileInput.current?.click()}>
              {avatar.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Camera aria-hidden="true"/>}
            </Button>
          </div>
          <div className="profile-menu-identity-copy">
            <small>Nome de exibição</small>
            {editingName ? (
              <div className="profile-name-edit">
                <Input aria-label="Nome de exibição" value={name} onChange={e => setName(e.target.value)} autoFocus
                       onKeyDown={e => {
                         if (e.key === 'Enter' && name.trim()) save.mutate({name: name.trim()});
                         if (e.key === 'Escape') setEditingName(false);
                       }}/>
                <Button size="icon" disabled={!name.trim() || save.isPending} aria-label="Salvar"
                        onClick={() => save.mutate({name: name.trim()})}>
                  {save.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Check aria-hidden="true"/>}
                </Button>
                <Button size="icon" variant="ghost" aria-label="Cancelar edição do nome"
                        onClick={() => setEditingName(false)}><X aria-hidden="true"/></Button>
              </div>
            ) : (
              <button type="button" className="profile-name-display" onClick={() => {
                setName(me?.name || '');
                setEditingName(true);
              }}>
                <span>{me?.name || 'Definir nome'}</span><Pencil aria-hidden="true"/>
              </button>
            )}
            <span className="profile-visibility"><i aria-hidden="true"/>
              {me?.showcase_public ? 'Vitrine pública' : 'Vitrine privada'}
            </span>
          </div>
          {me?.avatar_url && <Button type="button" size="icon" variant="ghost" disabled={removeAvatar.isPending}
                                     className="profile-avatar-remove" aria-label="Remover foto de perfil"
                                     onClick={() => removeAvatar.mutate()}>
            {removeAvatar.isPending ? <LoaderCircle className="spin" aria-hidden="true"/> :
              <Trash2 aria-hidden="true"/>}
          </Button>}
          <input ref={fileInput} className="sr-only" type="file" accept="image/jpeg,image/png"
                 aria-label="Selecionar foto de perfil" onChange={event => {
            const file = event.target.files?.[0];
            if (file) avatar.mutate(file);
            event.target.value = '';
          }}/>
        </header>

        <section className="profile-menu-section" aria-labelledby="profile-table-title">
          <div className="profile-menu-section-heading">
            <div><Sparkles aria-hidden="true"/><span><b id="profile-table-title">Sua mesa, do seu jeito</b>
              <small>Preferências aplicadas na próxima mão.</small></span></div>
          </div>
          <div className="profile-menu-setting">
            <span><Label id="wallet-mode-label">{walletMode === 'real' ? 'Dinheiro real' : 'Sandbox'}</Label>
              <small>Modo de jogo</small></span>
            <Switch aria-labelledby="wallet-mode-label"
                    checked={walletMode === 'real'}
                    onCheckedChange={checked => save.mutate({wallet_mode: checked ? 'real' : 'sandbox'})}/>
          </div>
          <div className="profile-deck-setting">
            <div className="profile-deck-label">
              <Label id="deck-variant-label">Baralho</Label>
              <span className="profile-deck-preview" aria-hidden="true">
                {ACES.map(card => <Image key={card} src={cardPath(card, deckVariant)} alt="" width={20} height={28}/>)}
              </span>
            </div>
            <Select value={deckVariant}
                    onValueChange={(value: DeckVariantId | null) => value && save.mutate({deck_variant: value})}>
              <SelectTrigger aria-labelledby="deck-variant-label" disabled={save.isPending}>
                <SelectValue>
                  {(value: DeckVariantId) => DECK_VARIANTS[value]?.label ?? DECK_VARIANTS[DEFAULT_DECK_VARIANT].label}
                </SelectValue>
              </SelectTrigger>
              <SelectContent className="profile-deck-options" align="end">
                {Object.entries(DECK_VARIANTS).map(([id, variant]) => (
                  <SelectItem key={id} value={id as DeckVariantId} label={variant.label}>
                    <span className="deck-variant-option">
                      <span className="deck-variant-option-cards">
                        {ACES.map(card => <Image key={card} src={cardPath(card, id as DeckVariantId)} alt=""
                                                 height={0} width={0} style={{width: '20px', height: 'auto'}}/>)}
                      </span>
                      {variant.label}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </section>

        <section className="profile-wallet" aria-label="Seus saldos">
          <div className="profile-wallet-heading"><WalletCards aria-hidden="true"/><b>Seus saldos</b></div>
          <div className="profile-balances">
            <span>Fichas sandbox <b>{formatSandbox(me?.sandbox_balance)}</b></span>
            <span>Dinheiro real <b>{formatReal(me?.game_balance)}</b></span>
          </div>
          <Button type="button" variant="ghost" className="profile-wallet-link" render={<Link href="/store"/>}>
            <ShoppingBag aria-hidden="true"/> <span><b>Loja</b><small>Reações e fichas sandbox</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
        </section>

        <nav className="profile-menu-links" aria-label="Detalhes do perfil">
          <Button type="button" variant="ghost" aria-label="Seu jogo" onClick={() => setSelfHudOpen(true)}>
            <Activity aria-hidden="true"/><span><b>Seu jogo</b><small>Estatísticas e estilo na mesa</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
          <Button type="button" variant="ghost" aria-label="Vitrine do perfil" onClick={() => setShowcaseOpen(true)}>
            <Eye aria-hidden="true"/><span><b>Vitrine do perfil</b><small>Escolha o que os outros veem</small></span>
            <ChevronRight aria-hidden="true"/>
          </Button>
        </nav>

        <Button variant="ghost" className="profile-menu-logout" onClick={() => logout()}>
          <LogOut aria-hidden="true"/> Sair da conta
        </Button>
      </div>
    </PopoverContent>
  </Popover>
    <ProfileShowcaseDialog open={showcaseOpen} onOpenChangeAction={setShowcaseOpen}/>
    <SelfHudDialog open={selfHudOpen} onOpenChangeAction={setSelfHudOpen}/>
  </>;
}
