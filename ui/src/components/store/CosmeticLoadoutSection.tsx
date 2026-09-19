'use client';
import type {RefObject} from 'react';
import {useRef, useState} from 'react';
import Image from 'next/image';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Bookmark, Check, LoaderCircle, PlayCircle, Sparkles, Trash2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {SkeletonList} from '@/components/ui/skeleton';
import {ApiError} from '@/lib/api/client';
import {
  applyCosmeticLoadout, createCosmeticLoadout, deleteCosmeticLoadout, listCosmeticLoadouts,
  MAX_COSMETIC_LOADOUTS, type CosmeticLoadout
} from '@/lib/api/cosmeticLoadouts';
import {BALANCE_QUERY_KEY} from '@/lib/api/player';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, DEFAULT_DECK_VARIANT, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';
import {pushNotification} from '@/lib/notify';

const ACES = ['As', 'Ah', 'Ad', 'Ac'];
// Mirrors player.DefaultTableTheme server-side (the "classic" entry) — same
// literal CosmeticPurchaseDialog's pairedItemId fallback already uses.
const DEFAULT_TABLE_THEME: TableThemeId = 'classic';

export const COSMETIC_LOADOUTS_QUERY_KEY = ['cosmetic-loadouts'] as const;

function deckLabel(id?: string) {
  return id ? DECK_VARIANTS[id as DeckVariantId]?.label ?? id : undefined;
}

function feltLabel(id?: string) {
  return id ? TABLE_THEMES[id as TableThemeId]?.label ?? id : undefined;
}

/** Same combo-swatch visual `CosmeticPurchaseDialog`'s preview uses — deck
 * over felt is decorative here too (the name and the two labels below it
 * already say what the combo is), so it stays `aria-hidden`. */
function LoadoutComboPreview({deckId, feltId}: { deckId?: string; feltId?: string }) {
  const theme = (feltId && TABLE_THEMES[feltId as TableThemeId]) || TABLE_THEMES[DEFAULT_TABLE_THEME];
  const knownDeck = deckId && DECK_VARIANTS[deckId as DeckVariantId];
  return <span className="cosmetic-preview-combo-visual" aria-hidden="true">
    <span className="felt-swatch"
          style={{'--theme-a': theme.colors[0], '--theme-b': theme.colors[1]} as React.CSSProperties}/>
    {knownDeck && <span className="cosmetic-store-deck-preview">
      {ACES.map(card => <Image key={card} src={cardPath(card, deckId as DeckVariantId)} alt="" width={28} height={40}/>)}
    </span>}
  </span>;
}

function comboSummary(selections: Record<string, string>) {
  const deck = deckLabel(selections.deck);
  const felt = feltLabel(selections.felt);
  return [deck, felt].filter(Boolean).join(' · ') || 'Combinação salva';
}

/**
 * "Salvar combinação atual" — opens with the player's live deck+felt already
 * shown as the preview, and only asks for a name. Disabled by the caller
 * once the player is at `MAX_COSMETIC_LOADOUTS` (#313), with the reason
 * spelled out in the button itself rather than only discovered on submit.
 */
function SaveLoadoutDialog({open, deckVariant, tableTheme, finalFocusRef, onCloseAction, onSavedAction}: {
  open: boolean;
  deckVariant?: string;
  tableTheme?: string;
  finalFocusRef?: RefObject<HTMLButtonElement | null>;
  onCloseAction: () => void;
  onSavedAction: (loadout: CosmeticLoadout) => void;
}) {
  const [name, setName] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');

  async function save() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setPending(true);
    setError('');
    try {
      const loadout = await createCosmeticLoadout(trimmed, {
        deck: deckVariant || DEFAULT_DECK_VARIANT, felt: tableTheme || DEFAULT_TABLE_THEME
      });
      onSavedAction(loadout);
      setName('');
    } catch {
      setError('Não foi possível salvar este combo agora. Tente novamente.');
    } finally {
      setPending(false);
    }
  }

  return <Dialog open={open} onOpenChange={isOpen => !isOpen && !pending && onCloseAction()}>
    <DialogContent className="cosmetic-loadout-dialog" finalFocus={finalFocusRef}>
      <DialogHeader>
        <span className="cosmetic-purchase-hero">
          <LoadoutComboPreview deckId={deckVariant} feltId={tableTheme}/>
        </span>
        <DialogTitle>Salvar combinação atual</DialogTitle>
        <DialogDescription>
          {deckLabel(deckVariant) || DECK_VARIANTS[DEFAULT_DECK_VARIANT].label} com{' '}
          {feltLabel(tableTheme) || TABLE_THEMES[DEFAULT_TABLE_THEME].label}. Dê um nome para aplicar os dois de uma vez depois.
        </DialogDescription>
      </DialogHeader>
      <div className="cosmetic-loadout-name-field">
        <Label htmlFor="cosmetic-loadout-name">Nome do combo</Label>
        <Input id="cosmetic-loadout-name" value={name} maxLength={60} disabled={pending} autoFocus
               placeholder="Ex.: Mesa de torneio" onChange={event => setName(event.target.value)}
               onKeyDown={event => event.key === 'Enter' && void save()}/>
      </div>
      {error && <p className="reaction-purchase-error" role="alert">{error}</p>}
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={pending} onClick={onCloseAction}>Cancelar</Button>
        <Button type="button" disabled={pending || !name.trim()} onClick={() => void save()}>
          {pending ? <LoaderCircle className="spin" aria-hidden="true"/> : <Bookmark aria-hidden="true"/>}
          {pending ? 'Salvando…' : 'Salvar combo'}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

/**
 * Applying or deleting a saved loadout, both gated behind the same
 * confirm/pending/success/error shape `CosmeticRefundDialog` uses. Apply
 * shows the combo's preview before anything is touched (#313's "preview
 * before confirming" acceptance) and reports the one way it can fail that
 * is the player's fault to fix, not a transient error: an item in the combo
 * stopped being owned (refunded) since it was saved.
 */
function LoadoutActionDialog({mode, loadout, finalFocusRef, onCloseAction, onConfirmAction}: {
  mode: 'apply' | 'delete';
  loadout: CosmeticLoadout | null;
  finalFocusRef?: RefObject<HTMLButtonElement | null>;
  onCloseAction: () => void;
  onConfirmAction: (loadout: CosmeticLoadout) => Promise<void>;
}) {
  const [state, setState] = useState<'confirm' | 'pending' | 'success' | 'error'>('confirm');
  const [error, setError] = useState('');
  if (!loadout) return null;

  async function confirm() {
    setState('pending');
    setError('');
    try {
      await onConfirmAction(loadout!);
      setState('success');
    } catch (caught) {
      setError(mode === 'apply'
        ? (caught instanceof ApiError && caught.status === 400
          ? 'Um item deste combo não é mais seu (talvez estornado). Salve um novo combo com os itens atuais.'
          : 'Não foi possível aplicar este combo agora. Tente novamente.')
        : 'Não foi possível excluir este combo agora. Tente novamente.');
      setState('error');
    }
  }

  const title = mode === 'apply'
    ? (state === 'success' ? 'Combo aplicado' : `Aplicar "${loadout.name}"?`)
    : (state === 'success' ? 'Combo excluído' : `Excluir "${loadout.name}"?`);

  return <Dialog open onOpenChange={isOpen => !isOpen && state !== 'pending' && onCloseAction()}>
    <DialogContent className="reaction-refund-dialog cosmetic-loadout-dialog" finalFocus={finalFocusRef}>
      <DialogHeader>
        {mode === 'apply' && state !== 'success' && <span className="cosmetic-purchase-hero">
          <LoadoutComboPreview deckId={loadout.selections.deck} feltId={loadout.selections.felt}/>
        </span>}
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{
          mode === 'apply'
            ? (state === 'success' ? 'Seu baralho e feltro já são estes em qualquer mesa.'
              : 'Isso troca seu baralho e feltro atuais pelos deste combo. Você pode aplicar outro combo quando quiser.')
            : (state === 'success' ? 'O combo não aparece mais na sua lista. Os itens continuam seus, só o combo saiu.'
              : 'O combo é removido; os itens salvos nele continuam seus e podem ser escolhidos avulsos ou salvos em outro combo.')
        }</DialogDescription>
      </DialogHeader>
      {state === 'success' ? <div className="reaction-refund-success" role="status">
        <Check aria-hidden="true"/>
        <div><strong>{mode === 'apply' ? 'Pronto para a mesa' : 'Combo removido'}</strong>
          <p>{mode === 'apply' ? 'Feche esta janela e volte para a mesa.' : 'Você ainda pode salvar um novo combo quando quiser.'}</p>
        </div>
      </div> : <>
        {mode === 'apply' && <dl className="reaction-refund-summary">
          <div><dt>Baralho</dt><dd>{deckLabel(loadout.selections.deck) || 'Sem alteração'}</dd></div>
          <div><dt>Feltro</dt><dd>{feltLabel(loadout.selections.felt) || 'Sem alteração'}</dd></div>
        </dl>}
        {error && <p className="reaction-purchase-error" role="alert">{error}</p>}
      </>}
      <DialogFooter>
        {state === 'success' ? <Button type="button" onClick={onCloseAction}>Concluir</Button> : <>
          <Button type="button" variant="ghost" disabled={state === 'pending'} onClick={onCloseAction}>
            {mode === 'apply' ? 'Agora não' : 'Manter combo'}
          </Button>
          <Button type="button" variant={mode === 'delete' ? 'destructive' : 'default'} disabled={state === 'pending'}
                  onClick={() => void confirm()}>
            {state === 'pending' ? <LoaderCircle className="spin" aria-hidden="true"/>
              : mode === 'apply' ? <PlayCircle aria-hidden="true"/> : <Trash2 aria-hidden="true"/>}
            {state === 'pending' ? 'Aplicando…' : mode === 'apply' ? 'Aplicar combo' : 'Excluir combo'}
          </Button>
        </>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

/**
 * The whole "Combos salvos" department (#313): the save action (current
 * deck+felt, named), the bounded (max 5) saved list with per-item
 * apply/delete, and every state the rest of the store's departments already
 * cover — loading skeleton, retry-on-error, and an empty state that explains
 * the feature instead of just being blank.
 */
export function CosmeticLoadoutSection({deckVariant, tableTheme, seen, sectionRef}: {
  deckVariant?: string;
  tableTheme?: string;
  seen: boolean;
  /** From `useInViewOnce` — a callback ref, not a `RefObject`. */
  sectionRef: (node: Element | null) => void;
}) {
  const queryClient = useQueryClient();
  const [saveOpen, setSaveOpen] = useState(false);
  const [actionTarget, setActionTarget] = useState<{ mode: 'apply' | 'delete'; loadout: CosmeticLoadout } | null>(null);
  // One shared slot, same pattern the store page itself uses: only one
  // dialog is ever open, and every trigger writes here on the way in so
  // focus returns to it on close.
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  const loadouts = useQuery({
    queryKey: COSMETIC_LOADOUTS_QUERY_KEY, queryFn: listCosmeticLoadouts, enabled: seen
  });
  const items = loadouts.data ?? [];
  const atLimit = items.length >= MAX_COSMETIC_LOADOUTS;

  function invalidateLoadouts() {
    return queryClient.invalidateQueries({queryKey: COSMETIC_LOADOUTS_QUERY_KEY});
  }

  return <section id="loadouts" ref={sectionRef} className="store-section store-department cosmetic-loadout-section"
                  aria-labelledby="cosmetic-loadouts-title">
    <div className="store-section-heading">
      <Bookmark aria-hidden="true"/>
      <div><h2 id="cosmetic-loadouts-title">Combos salvos</h2>
        <p>Salve baralho e feltro juntos com um nome e aplique os dois de uma vez na próxima mesa.</p></div>
    </div>

    <div className="cosmetic-loadout-save-row">
      <div className="cosmetic-loadout-save-preview" aria-hidden="true">
        <LoadoutComboPreview deckId={deckVariant} feltId={tableTheme}/>
      </div>
      <span className="cosmetic-loadout-save-copy">
        <strong>Combinação atual</strong>
        <small>{deckLabel(deckVariant) || DECK_VARIANTS[DEFAULT_DECK_VARIANT].label} · {feltLabel(tableTheme) || TABLE_THEMES[DEFAULT_TABLE_THEME].label}</small>
      </span>
      <Button type="button" size="sm" disabled={atLimit}
              onClick={event => {
                triggerRef.current = event.currentTarget;
                setSaveOpen(true);
              }}>
        <Bookmark aria-hidden="true"/> Salvar como combo
      </Button>
    </div>
    {atLimit && <p className="cosmetic-loadout-limit-note">
      Você já tem {MAX_COSMETIC_LOADOUTS} combos salvos, o máximo. Exclua um combo para salvar outro.
    </p>}

    {loadouts.isLoading
      ? <SkeletonList label="Carregando seus combos…" count={2} height={72} className="cosmetic-loadout-list"/>
      : loadouts.isError
        ? <div className="lobby-empty">Não foi possível carregar seus combos agora.
          <Button variant="outline" size="sm" onClick={() => void loadouts.refetch()}>Tentar novamente</Button>
        </div>
        : items.length === 0
          ? <div className="lobby-empty"><Sparkles aria-hidden="true"/>
            <p>Nenhum combo salvo ainda. Escolha um baralho e um feltro e salve a combinação acima.</p>
          </div>
          : <ul className="cosmetic-loadout-list" aria-label="Seus combos salvos">
            {items.map(loadout => <li key={loadout.id} className="cosmetic-loadout-item">
              <span className="cosmetic-loadout-preview" aria-hidden="true">
                <LoadoutComboPreview deckId={loadout.selections.deck} feltId={loadout.selections.felt}/>
              </span>
              <span className="cosmetic-loadout-copy">
                <strong>{loadout.name}</strong>
                <small>{comboSummary(loadout.selections)}</small>
              </span>
              <span className="cosmetic-loadout-actions">
                <Button type="button" size="sm" onClick={event => {
                  triggerRef.current = event.currentTarget;
                  setActionTarget({mode: 'apply', loadout});
                }}>
                  <PlayCircle aria-hidden="true"/> Aplicar
                </Button>
                <Button type="button" variant="ghost" size="sm" aria-label={`Excluir combo ${loadout.name}`}
                        onClick={event => {
                          triggerRef.current = event.currentTarget;
                          setActionTarget({mode: 'delete', loadout});
                        }}>
                  <Trash2 aria-hidden="true"/>
                </Button>
              </span>
            </li>)}
          </ul>}

    <SaveLoadoutDialog open={saveOpen} deckVariant={deckVariant} tableTheme={tableTheme}
                       finalFocusRef={triggerRef}
                       onCloseAction={() => setSaveOpen(false)}
                       onSavedAction={async () => {
                         setSaveOpen(false);
                         await invalidateLoadouts();
                         pushNotification('Combo salvo. Ele já aparece na sua lista.', 'info');
                       }}/>
    <LoadoutActionDialog key={`${actionTarget?.mode ?? 'closed'}:${actionTarget?.loadout.id ?? ''}`}
                         mode={actionTarget?.mode ?? 'apply'} loadout={actionTarget?.loadout ?? null}
                         finalFocusRef={triggerRef}
                         onCloseAction={() => setActionTarget(null)}
                         onConfirmAction={async target => {
                           if (actionTarget?.mode === 'delete') {
                             await deleteCosmeticLoadout(target.id);
                             await invalidateLoadouts();
                           } else {
                             await applyCosmeticLoadout(target.id);
                             await queryClient.invalidateQueries({queryKey: BALANCE_QUERY_KEY});
                           }
                         }}/>
  </section>;
}
