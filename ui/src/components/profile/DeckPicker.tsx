'use client';
import {useId} from 'react';
import Image from 'next/image';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {LockKeyhole} from 'lucide-react';
import {Label} from '@/components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {listCosmeticCatalog, ownedCosmeticIDs} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId, DEFAULT_DECK_VARIANT, PREMIUM_DECK_IDS} from '@/lib/cardVariants';
import {useTablePreferenceSave} from '@/lib/hooks/useProfileEdits';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

/**
 * The whole deck setting: label, four-ace preview, the variant list with its
 * premium locks, and the write. Both the header popover and `/player-profile`
 * render this exact component, so ownership, the locked-item store link and
 * the confirmation copy have one implementation.
 *
 * It reads `['wallet','cosmetic-catalog','deck']` on mount, which is why the
 * popover mounts it only once opened: that menu is in every authenticated
 * page's chrome and must not spend the read on a player who never looks at it
 * (#232). The same reason keeps this module out of the popover's static import
 * graph (see `ProfileMenu`): the `Select` primitive and the variant catalogue
 * are first-load JS every one of those routes would otherwise pay for.
 *
 * Every id is generated, because the popover can be open on top of the route:
 * two pickers on screen must not share one label id.
 */
export function DeckPicker({deckVariant, className}: {deckVariant?: DeckVariantId; className: string}) {
  const labelId = useId();
  const {data: deckCatalog = [], isLoading: deckCatalogLoading} = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'deck'], queryFn: () => listCosmeticCatalog('deck')
  });
  const ownedDecks = ownedCosmeticIDs(deckCatalog);
  const deckPrices = new Map(deckCatalog.map(entry => [entry.id, entry.price_fichas]));
  const save = useTablePreferenceSave();
  const variant: DeckVariantId = deckVariant || DEFAULT_DECK_VARIANT;

  return <div className={className}>
    <div className="profile-deck-label">
      <Label id={labelId}>Baralho</Label>
      <span className="profile-deck-preview" aria-hidden="true">
        {ACES.map(card => <Image key={card} src={cardPath(card, variant)} alt="" width={20} height={28}/>)}
      </span>
    </div>
    <Select value={variant} onValueChange={(value: DeckVariantId | null) => {
      if (!value) return;
      // Locked items render as a Link to the store instead (below) and never
      // reach this branch on a real click, but guard the value change too in
      // case selection is ever driven by keyboard/programmatically.
      if (PREMIUM_DECK_IDS.has(value) && !ownedDecks.has(value)) return;
      save.mutate({deck_variant: value});
    }}>
      <SelectTrigger aria-labelledby={labelId} disabled={save.isPending}>
        <SelectValue>
          {(value: DeckVariantId) => DECK_VARIANTS[value]?.label ?? DECK_VARIANTS[DEFAULT_DECK_VARIANT].label}
        </SelectValue>
      </SelectTrigger>
      <SelectContent className="profile-deck-options" align="end">
        {Object.entries(DECK_VARIANTS).map(([id, deck]) => {
          const premium = PREMIUM_DECK_IDS.has(id as DeckVariantId);
          // Same beat as the felt picker: until the catalog lands a premium
          // deck is neither known-locked nor selectable, so it waits in the
          // Select's disabled state instead of flashing a padlock (and a
          // store link) at a player who already owns it.
          const locked = premium && !deckCatalogLoading && !ownedDecks.has(id as DeckVariantId);
          const price = deckPrices.get(id);
          return <SelectItem key={id} value={id as DeckVariantId} label={deck.label}
                             disabled={premium && deckCatalogLoading}
                             {...(locked ? {render: <Link href="/store#decks"/>} : {})}>
            <span className={`deck-variant-option${locked ? ' locked' : ''}`}>
              <span className="deck-variant-option-cards">
                {ACES.map(card => <Image key={card} src={cardPath(card, id as DeckVariantId)} alt=""
                                         height={0} width={0} style={{width: '20px', height: 'auto'}}/>)}
              </span>
              {deck.label}
              {locked && <LockKeyhole aria-label={`Baralho premium bloqueado${
                price ? ` · ${price.toLocaleString('pt-BR')} fichas` : ''} · Ver na loja`}/>}
            </span>
          </SelectItem>;
        })}
      </SelectContent>
    </Select>
  </div>;
}
