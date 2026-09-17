'use client';
import {useState} from 'react';
import Link from 'next/link';
import {useMutation, useQueryClient} from '@tanstack/react-query';
import {ArrowDown, ArrowUp, Copy, Eye, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Switch} from '@/components/ui/switch';
import {SkeletonList} from '@/components/ui/skeleton';
import {FeaturedAchievementCarousel} from '@/components/achievements/FeaturedAchievementCarousel';
import {useAchievementsSummary} from '@/lib/hooks/useAchievementsSummary';
import {
  normalizeShowcaseLayout, type PlayerProfile, type ShowcaseLayout, type ShowcaseSectionId, updateMe
} from '@/lib/api/player';
import {pushNotification} from '@/lib/notify';

export const MAX_FEATURED = 3;

const SHOWCASE_SECTION_LABELS: Record<ShowcaseSectionId, string> = {
  achievements: 'Conquistas em Destaque', best_hand: 'Melhor Vitória Recente', matchup: 'Cara a Cara'
};
// Achievements can be reordered but never hidden — it already has its own
// "nenhuma conquista selecionada" empty copy, so hiding it entirely would
// just duplicate that with less explanation.
const HIDEABLE_SECTIONS = new Set<ShowcaseSectionId>(['best_hand', 'matchup']);

export function profileHref(userId: string, preview = false) {
  return `/profile?id=${encodeURIComponent(userId)}${preview ? '&preview=1' : ''}`;
}

/**
 * Everything the public showcase is made of, on one screen: who can open it,
 * which achievements lead it, and the order its sections appear in. Privacy,
 * highlights and layout all land in a single `POST /players/me`, so they share
 * one save instead of three competing ones.
 */
export function ShowcaseSection({me}: {me: PlayerProfile}) {
  const queryClient = useQueryClient();
  const [isPublic, setIsPublic] = useState(me.showcase_public);
  const [isPlaystylePublic, setIsPlaystylePublic] = useState(me.playstyle_public);
  const [isTablePublic, setIsTablePublic] = useState(me.table_public);
  const [selected, setSelected] = useState<string[]>(me.featured_achievements || []);
  const [layout, setLayout] = useState<ShowcaseLayout>(() => normalizeShowcaseLayout(me.showcase_layout));
  const [layoutAnnouncement, setLayoutAnnouncement] = useState('');

  // The public showcase reads the player's chip-mode progress server-side, so
  // the picker has to offer the same set. Full-state summary (#79): the
  // paginated endpoint silently dropped any key past page one.
  const mine = useAchievementsSummary('sandbox', true);
  const earned = (mine.data?.achievements || []).filter(entry => entry.progress > 0);

  const save = useMutation({
    mutationFn: () => updateMe({
      showcase_public: isPublic, playstyle_public: isPlaystylePublic, table_public: isTablePublic,
      featured_achievements: selected, showcase_layout: layout
    }),
    onSuccess: profile => {
      queryClient.setQueryData(['player', 'me'], profile);
      pushNotification('Vitrine do perfil atualizada.', 'info');
    },
    onError: () => pushNotification('Não foi possível salvar a vitrine. Tente novamente.', 'error')
  });

  function moveSection(id: ShowcaseSectionId, direction: -1 | 1) {
    setLayout(current => {
      const index = current.order.indexOf(id);
      const target = index + direction;
      if (target < 0 || target >= current.order.length) return current;
      const order = [...current.order];
      [order[index], order[target]] = [order[target], order[index]];
      setLayoutAnnouncement(`${SHOWCASE_SECTION_LABELS[id]} agora em ${target + 1}º lugar de ${order.length}.`);
      return {...current, order};
    });
  }

  function toggleSectionVisible(id: ShowcaseSectionId, visible: boolean) {
    setLayout(current => ({
      ...current,
      hidden: visible ? current.hidden.filter(item => item !== id) : [...current.hidden, id]
    }));
  }

  function toggleFeatured(key: string) {
    setSelected(current => {
      if (current.includes(key)) return current.filter(item => item !== key);
      if (current.length >= MAX_FEATURED) {
        pushNotification(`Escolha no máximo ${MAX_FEATURED} conquistas.`, 'info');
        return current;
      }
      // Newly highlighted goes to the front, which is where the carousel and
      // the public showcase both read the order from.
      return [key, ...current];
    });
  }

  const shareUrl = typeof window === 'undefined'
    ? profileHref(me.user_id)
    : `${window.location.origin}${profileHref(me.user_id)}`;

  return <section className="player-profile-section" aria-labelledby="player-profile-showcase-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-showcase-title">Sua vitrine</h2>
      <p>O perfil público que abre por link. Começa privada, e só você decide quando mostrar.</p>
    </header>

    <fieldset className="showcase-privacy-group">
      <legend>Quem pode ver</legend>
      <div className="showcase-privacy-row">
        <span><b>Vitrine pública</b><small>Permite abrir a vitrine por link.</small></span>
        <Switch checked={isPublic} onCheckedChange={setIsPublic} aria-label="Vitrine pública"/>
      </div>
      <div className="showcase-privacy-row">
        <span><b>Estilo de jogo</b><small>Após 200 mãos, exibe um rótulo de tendência na mesa e na vitrine.</small></span>
        <Switch checked={isPlaystylePublic} onCheckedChange={setIsPlaystylePublic} disabled={!isPublic}
                aria-label="Estilo de jogo público"/>
      </div>
      <div className="showcase-privacy-row">
        <span><b>Mesa visível para amigos</b><small>Amigos podem entrar na sua mesa quando ela for pública.</small></span>
        <Switch checked={isTablePublic} onCheckedChange={setIsTablePublic} aria-label="Mesa visível para amigos"/>
      </div>
    </fieldset>

    <div className="player-profile-featured">
      <h3 id="player-profile-featured-title">
        <Trophy aria-hidden="true"/> Conquistas em destaque
        <span className="player-profile-featured-count">{selected.length}/{MAX_FEATURED}</span>
      </h3>
      <p>Toque numa conquista para destacá-la. Ela passa para o começo da fila, e as demais seguem por estrelas.</p>
      {mine.isLoading
        ? <SkeletonList label="Carregando suas conquistas…" count={3} height={112} className="skeleton-panel"/>
        : earned.length === 0
          ? <p className="player-profile-empty">
            Você ainda não pontuou em nenhuma conquista. Jogue algumas mãos e elas aparecem aqui.{' '}
            <Link href="/achievements">Ver o catálogo</Link>
          </p>
          : <FeaturedAchievementCarousel entries={earned} selected={selected} max={MAX_FEATURED}
                                         onToggleAction={toggleFeatured}/>}
    </div>

    <div className="player-profile-order">
      <h3 id="player-profile-order-title">Ordem das seções</h3>
      <p>Melhor Vitória e Cara a Cara também podem ficar fora da vitrine.</p>
      <p className="sr-only" role="status" aria-live="polite">{layoutAnnouncement}</p>
      <ol className="showcase-layout-list" aria-labelledby="player-profile-order-title">
        {layout.order.map((id, index) => <li key={id}>
          <span aria-label={`${SHOWCASE_SECTION_LABELS[id]}, posição ${index + 1} de ${layout.order.length}`}>
            {SHOWCASE_SECTION_LABELS[id]}
          </span>
          <span className="showcase-layout-controls">
            <Button type="button" variant="ghost" size="icon" disabled={index === 0}
                    aria-label={`Mover ${SHOWCASE_SECTION_LABELS[id]} para cima`}
                    onClick={() => moveSection(id, -1)}><ArrowUp aria-hidden="true"/></Button>
            <Button type="button" variant="ghost" size="icon" disabled={index === layout.order.length - 1}
                    aria-label={`Mover ${SHOWCASE_SECTION_LABELS[id]} para baixo`}
                    onClick={() => moveSection(id, 1)}><ArrowDown aria-hidden="true"/></Button>
            {HIDEABLE_SECTIONS.has(id) && <Switch checked={!layout.hidden.includes(id)}
                                                  onCheckedChange={checked => toggleSectionVisible(id, checked)}
                                                  aria-label={`Mostrar ${SHOWCASE_SECTION_LABELS[id]} na vitrine`}/>}
          </span>
        </li>)}
      </ol>
    </div>

    <div className="player-profile-showcase-actions">
      <Button type="button" loading={save.isPending} onClick={() => save.mutate()}>
        {save.isPending ? 'Salvando…' : 'Salvar vitrine'}
      </Button>
      <Button type="button" variant="outline" render={<Link href={profileHref(me.user_id, true)}/>}>
        <Eye aria-hidden="true"/> Ver como visitante
      </Button>
      {me.showcase_public && <Button type="button" variant="ghost" onClick={async () => {
        await navigator.clipboard.writeText(shareUrl);
        pushNotification('Link do perfil copiado.', 'info');
      }}><Copy aria-hidden="true"/> Copiar link</Button>}
    </div>
  </section>;
}
