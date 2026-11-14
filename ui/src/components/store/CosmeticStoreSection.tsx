'use client';
import type {ReactNode} from 'react';
import Image from 'next/image';
import Link from 'next/link';
import {LockKeyhole, QrCode, RotateCcw, Sparkles, UserRoundPen} from 'lucide-react';
import {Button, SkeletonList} from '@aoctech/ui';
import type {AvatarStoreKind, CosmeticCatalogEntry, CosmeticKind, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';
import type {ActivityRow} from '@/components/store/PurchaseActivityList';
import {currentCosmeticPurchase} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';
import {moneyExact} from '@/lib/chips';
import {AVATAR_ITEMS} from '@/lib/avatarCosmetics';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {FRAME_ART} from '@/components/avatar/frames';
import {HAT_ART} from '@/components/avatar/hats';
import {ACCESSORY_ART} from '@/components/avatar/accessories';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

const STATUS_LABEL: Record<string, string> = {
  processing: 'Processando', pending: 'Aguardando Pix', confirmed: 'Liberada', refunding: 'Estornando',
  refunded: 'Estornada', expired: 'Expirada', failed: 'Falhou'
};


export interface CosmeticSectionProps {
  catalog: CosmeticCatalogEntry[];
  purchases: CosmeticPurchase[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  // Each action receives the button that triggered it: the dialogs are opened
  // programmatically, so base-ui has nothing to restore focus to on close
  // unless we hand it the trigger ourselves.
  onBuyAction: (entry: CosmeticCatalogEntry, trigger: HTMLButtonElement) => void;
  onRefundAction: (purchase: CosmeticPurchase, trigger: HTMLButtonElement) => void;
  onResumeAction: (entry: CosmeticCatalogEntry, purchase: CosmeticPurchase, trigger: HTMLButtonElement) => void;
}

function CosmeticGrid({kind, labelFor, renderPreview, ariaLabel, loadingLabel, emptyLabel, equipHref, catalog, purchases,
                       isLoading, isError, onRetryAction, onBuyAction, onRefundAction, onResumeAction}: CosmeticSectionProps & {
  kind: CosmeticKind;
  labelFor: (id: string) => string | undefined;
  renderPreview: (id: string) => ReactNode;
  ariaLabel: string;
  loadingLabel: string;
  emptyLabel: string;
  /** Where an owned item is equipped, when that is not the table itself (avatar items: the profile). */
  equipHref?: string;
}) {
  if (isLoading) return <SkeletonList label={loadingLabel} count={4} height={96} className="cosmetic-store-grid"/>;
  if (isError) return <div className="lobby-empty">Não foi possível carregar o catálogo agora.
    <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button></div>;

  // Always-visible gallery: every catalog entry for this kind, owned and unowned, free and premium.
  const entries = catalog.filter(entry => entry.kind === kind && labelFor(entry.id));
  if (!entries.length) return <div className="lobby-empty"><Sparkles aria-hidden="true"/><p>{emptyLabel}</p></div>;

  return <ul className="cosmetic-store-grid" aria-label={ariaLabel}>
    {entries.map(entry => {
      const purchase = entry.premium ? currentCosmeticPurchase(purchases, entry.id) : undefined;
      // Ownership is the catalog's server-side entitlement flag (always true for
      // free items); the purchase row is only what a refund needs in order to
      // name a receipt. The two can disagree — history is paginated, and a
      // refunded item leaves rows behind — and ownership wins when they do.
      const owned = entry.owned;
      const active = purchase?.status === 'pending' || purchase?.status === 'processing';
      const refunding = purchase?.status === 'refunding';
      // Avatar kinds only; deck/felt never set source or available. An earned
      // item is never priced or sold, and a closed sale window stops new
      // purchases but never a Pix already under way (active wins above it).
      const earnable = entry.source === 'achievement' || entry.source === 'season';
      const unavailable = !owned && !active && !refunding && (earnable || entry.available === false);
      return <li key={entry.id} className={`cosmetic-store-item${owned ? ' owned' : ''}`}>
        <span className="cosmetic-store-preview" aria-hidden="true">{renderPreview(entry.id)}</span>
        <span className="cosmetic-store-copy"><strong>{labelFor(entry.id)}</strong>
          {entry.premium && !unavailable && <small>{moneyExact(entry.price_cents)} <span aria-hidden="true">·</span>
            {(entry.price_fichas ?? 0).toLocaleString('pt-BR')} fichas</small>}</span>
        {!entry.premium ? <span className="cosmetic-store-free">Grátis</span>
          : owned ? <span className="cosmetic-store-owned"><Sparkles aria-hidden="true"/> Sua</span>
            : active || refunding ? <span className="cosmetic-store-state">{STATUS_LABEL[purchase!.status]}</span>
              : earnable ? <span className="cosmetic-store-lock"><LockKeyhole aria-hidden="true"/> Conquista</span>
                : unavailable ? <span className="cosmetic-store-state">Indisponível</span>
                  : <span className="cosmetic-store-lock"><LockKeyhole aria-hidden="true"/> Não liberada</span>}
        <span className="cosmetic-store-action">
          {!entry.premium ? null
            : owned ? <>
                {equipHref && <Link className="cosmetic-store-equip" href={equipHref}>
                  <UserRoundPen aria-hidden="true"/> Equipar no perfil</Link>}
                {purchase && <Button type="button" variant="ghost" size="sm"
                                     onClick={event => onRefundAction(purchase, event.currentTarget)}>
                  <RotateCcw aria-hidden="true"/> Estornar</Button>}</>
              : refunding ? <span className="cosmetic-store-wait">Aguarde</span>
                : active ? <Button type="button" variant="outline" size="sm"
                                   onClick={event => onResumeAction(entry, purchase!, event.currentTarget)}>
                  <QrCode aria-hidden="true"/> Acompanhar</Button>
                : unavailable ? null
                : <Button type="button" size="sm" onClick={event => onBuyAction(entry, event.currentTarget)}>Liberar</Button>}
        </span>
      </li>;
    })}
  </ul>;
}

export function DeckStoreSection(props: CosmeticSectionProps) {
  return <CosmeticGrid {...props} kind="deck" labelFor={id => DECK_VARIANTS[id as DeckVariantId]?.label}
    renderPreview={id => <span className="cosmetic-store-deck-preview">
      {ACES.map(card => <Image key={card} src={cardPath(card, id as DeckVariantId)} alt="" width={14} height={20}/>)}
    </span>}
    ariaLabel="Catálogo de baralhos" loadingLabel="Carregando baralhos…"
    emptyLabel="Nenhum baralho disponível no momento."/>;
}

export function FeltStoreSection(props: CosmeticSectionProps) {
  return <CosmeticGrid {...props} kind="felt" labelFor={id => TABLE_THEMES[id as TableThemeId]?.label}
    renderPreview={id => {
      const theme = TABLE_THEMES[id as TableThemeId];
      return <span className="felt-swatch"
                   style={{'--theme-a': theme?.colors[0], '--theme-b': theme?.colors[1]} as React.CSSProperties}/>;
    }}
    ariaLabel="Catálogo de feltros" loadingLabel="Carregando feltros…"
    emptyLabel="Nenhum feltro disponível no momento."/>;
}

const AVATAR_ART = {frame: FRAME_ART, hat: HAT_ART, accessory: ACCESSORY_ART};
const AVATAR_KIND_LABEL: Record<AvatarStoreKind, string> = {frame: 'Borda', hat: 'Chapéu', accessory: 'Acessório'};

export const isAvatarStoreKind = (kind: CosmeticKind): kind is AvatarStoreKind => kind in AVATAR_ART;

/** The pt-BR label of any sold cosmetic, or undefined when this build has no
 * label (and so no art) for the id: callers hide such entries. */
export function cosmeticItemLabel(kind: CosmeticKind, id: string): string | undefined {
  if (kind === 'deck') return DECK_VARIANTS[id as DeckVariantId]?.label;
  if (kind === 'felt') return TABLE_THEMES[id as TableThemeId]?.label;
  return AVATAR_ART[kind][id] ? AVATAR_ITEMS[kind][id]?.label : undefined;
}

/** An avatar item worn by a neutral "EU" chip: the same `PlayerAvatar`
 * composition the table and profile use, never a second one. Decorative. */
export function AvatarItemPreview({kind, id, size}: { kind: AvatarStoreKind; id: string; size: number }) {
  const slot = {frame: {frameId: id}, hat: {hatId: id}, accessory: {accessoryId: id}}[kind];
  return <PlayerAvatar isViewer decorative size={size} {...slot}/>;
}

function avatarSection(kind: AvatarStoreKind, noun: string, plural: string) {
  return function AvatarStoreSection(props: CosmeticSectionProps) {
    return <CosmeticGrid {...props} kind={kind} labelFor={id => cosmeticItemLabel(kind, id)}
      renderPreview={id => <AvatarItemPreview kind={kind} id={id} size={48}/>} equipHref="/player-profile"
      ariaLabel={`Catálogo de ${plural}`} loadingLabel={`Carregando ${plural}…`}
      emptyLabel={`Nenhum${noun} disponível no momento.`}/>;
  };
}

export const FrameStoreSection = avatarSection('frame', 'a moldura', 'molduras');
export const HatStoreSection = avatarSection('hat', ' chapéu', 'chapéus');
export const AccessoryStoreSection = avatarSection('accessory', ' acessório', 'acessórios');

// Maps cosmetic purchases into the shared activity-list row shape. Ids never
// collide across kinds that matter here, and each row is labelled by its kind.
export function cosmeticActivityRows(purchases: CosmeticPurchase[]): ActivityRow[] {
  return purchases.map(item => {
    const label = cosmeticItemLabel(item.kind, item.item_id);
    const date = formatDate(item.updated_at || item.created_at);
    const kindLabel = isAvatarStoreKind(item.kind) ? AVATAR_KIND_LABEL[item.kind]
      : item.kind === 'deck' ? 'Baralho' : 'Feltro';
    return {
      id: item.purchase_id,
      label: label || item.item_id,
      detail: `${kindLabel} · ${item.method === 'pix' ? 'Pix' : 'Fichas'}${date ? ` · ${date}` : ''}`,
      status: item.status,
      statusLabel: STATUS_LABEL[item.status] || 'Atualizando',
      media: isAvatarStoreKind(item.kind) ? <AvatarItemPreview kind={item.kind} id={item.item_id} size={25}/>
        : item.kind === 'deck'
        ? <Image src={cardPath('As', item.item_id as DeckVariantId)} alt="" width={18} height={25}/>
        : <span className="felt-swatch" style={{
          '--theme-a': TABLE_THEMES[item.item_id as TableThemeId]?.colors[0],
          '--theme-b': TABLE_THEMES[item.item_id as TableThemeId]?.colors[1],
        } as React.CSSProperties}/>,
      at: item.updated_at || item.created_at || '',
    };
  });
}

function formatDate(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('pt-BR', {day: '2-digit', month: 'short'});
}
