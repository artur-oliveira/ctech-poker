'use client';
import Image from 'next/image';
import Link from 'next/link';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {LockKeyhole} from 'lucide-react';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {listCosmeticCatalog, ownedCosmeticIDs} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId, DEFAULT_DECK_VARIANT, PREMIUM_DECK_IDS} from '@/lib/cardVariants';
import {type PlayerProfile, updateMe, type WalletMode} from '@/lib/api/player';
import {availableWalletMode, REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';
import {pushNotification} from '@/lib/notify';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

/** Cosmetics and mode: what the next hand is dealt with. */
export function TableSection({me}: {me: PlayerProfile}) {
  const queryClient = useQueryClient();
  const {data: deckCatalog = [], isLoading: deckCatalogLoading} = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'deck'], queryFn: () => listCosmeticCatalog('deck')
  });
  const ownedDecks = ownedCosmeticIDs(deckCatalog);
  const deckPrices = new Map(deckCatalog.map(entry => [entry.id, entry.price_fichas]));

  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: (profile, input) => {
      queryClient.setQueryData(['player', 'me'], profile);
      if (input?.deck_variant) pushNotification('Baralho pronto para a próxima mão.', 'info');
      if (input?.wallet_mode) pushNotification(
        input.wallet_mode === 'real' ? 'Modo dinheiro real selecionado.' : 'Modo fichas selecionado.', 'info'
      );
    },
    onError: (_error, input) => {
      // A rejected wallet-mode change leaves the profile untouched; re-sync
      // from the server so the Switch snaps back to the real mode and tell the
      // player nothing changed (the generic API toast doesn't say which mode).
      if (input?.wallet_mode) {
        void queryClient.invalidateQueries({queryKey: ['player', 'me']});
        pushNotification('Não foi possível trocar o modo de jogo. Seu modo atual foi mantido.');
      }
    }
  });

  // Coerce to chips whenever the real-money UI is gated off, so nothing here
  // implies real money is active.
  const walletMode: WalletMode = availableWalletMode(me.wallet_mode);
  const deckVariant: DeckVariantId = me.deck_variant || DEFAULT_DECK_VARIANT;

  return <section className="player-profile-section" aria-labelledby="player-profile-table-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-table-title">Sua mesa</h2>
      <p>Preferências aplicadas na próxima mão.</p>
    </header>

    {REAL_MONEY_UI_ENABLED && <div className="player-profile-setting">
      <span><Label id="wallet-mode-label">{walletMode === 'real' ? 'Dinheiro real' : 'Fichas'}</Label>
        <small>Modo de jogo</small></span>
      <Switch aria-labelledby="wallet-mode-label" checked={walletMode === 'real'}
              onCheckedChange={checked => save.mutate({wallet_mode: checked ? 'real' : 'sandbox'})}/>
    </div>}

    <div className="player-profile-deck">
      <div className="profile-deck-label">
        <Label id="deck-variant-label">Baralho</Label>
        <span className="profile-deck-preview" aria-hidden="true">
          {ACES.map(card => <Image key={card} src={cardPath(card, deckVariant)} alt="" width={20} height={28}/>)}
        </span>
      </div>
      <Select value={deckVariant} onValueChange={(value: DeckVariantId | null) => {
        if (!value) return;
        // Locked items render as a Link to the store instead (below) and never
        // reach this branch on a real click, but guard the value change too in
        // case selection is ever driven by keyboard/programmatically.
        if (PREMIUM_DECK_IDS.has(value) && !ownedDecks.has(value)) return;
        save.mutate({deck_variant: value});
      }}>
        <SelectTrigger aria-labelledby="deck-variant-label" disabled={save.isPending}>
          <SelectValue>
            {(value: DeckVariantId) => DECK_VARIANTS[value]?.label ?? DECK_VARIANTS[DEFAULT_DECK_VARIANT].label}
          </SelectValue>
        </SelectTrigger>
        <SelectContent className="profile-deck-options" align="end">
          {Object.entries(DECK_VARIANTS).map(([id, variant]) => {
            const premium = PREMIUM_DECK_IDS.has(id as DeckVariantId);
            // Same beat as the felt picker: until the catalog lands a premium
            // deck is neither known-locked nor selectable, so it waits in the
            // Select's disabled state instead of flashing a padlock (and a
            // store link) at a player who already owns it.
            const locked = premium && !deckCatalogLoading && !ownedDecks.has(id as DeckVariantId);
            const price = deckPrices.get(id);
            return <SelectItem key={id} value={id as DeckVariantId} label={variant.label}
                               disabled={premium && deckCatalogLoading}
                               {...(locked ? {render: <Link href="/store#decks"/>} : {})}>
              <span className={`deck-variant-option${locked ? ' locked' : ''}`}>
                <span className="deck-variant-option-cards">
                  {ACES.map(card => <Image key={card} src={cardPath(card, id as DeckVariantId)} alt=""
                                           height={0} width={0} style={{width: '20px', height: 'auto'}}/>)}
                </span>
                {variant.label}
                {locked && <LockKeyhole aria-label={`Baralho premium bloqueado${
                  price ? ` · ${price.toLocaleString('pt-BR')} fichas` : ''} · Ver na loja`}/>}
              </span>
            </SelectItem>;
          })}
        </SelectContent>
      </Select>
    </div>
  </section>;
}
