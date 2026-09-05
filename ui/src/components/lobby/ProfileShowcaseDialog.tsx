'use client';
import {useState} from 'react';
import Link from 'next/link';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {ArrowDown, ArrowUp, Copy, ExternalLink} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog';
import {Switch} from '@/components/ui/switch';
import {getAchievementCatalog} from '@/lib/api/achievements';
import {
  getMe, normalizeShowcaseLayout, type PlayerProfile, type ShowcaseLayout, type ShowcaseSectionId, updateMe
} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';
import {pushNotification} from '@/lib/notify';
import {SkeletonList} from '@/components/ui/skeleton';
import {useAchievementsSummary} from '@/lib/hooks/useAchievementsSummary';

const SHOWCASE_SECTION_LABELS: Record<ShowcaseSectionId, string> = {
  achievements: 'Conquistas em Destaque', best_hand: 'Melhor Vitória Recente', matchup: 'Cara a Cara'
};
// Achievements can be reordered but never hidden — it already has its own
// "nenhuma conquista selecionada" empty copy, so hiding it entirely would
// just duplicate that with less explanation.
const HIDEABLE_SECTIONS = new Set<ShowcaseSectionId>(['best_hand', 'matchup']);

function ShowcaseEditor({me, onSaved}: { me: PlayerProfile; onSaved: (profile: PlayerProfile) => void }) {
  const [isPublic, setIsPublic] = useState(me.showcase_public);
  const [isPlaystylePublic, setIsPlaystylePublic] = useState(me.playstyle_public);
  const [isTablePublic, setIsTablePublic] = useState(me.table_public);
  const [selected, setSelected] = useState<string[]>(me.featured_achievements || []);
  const [layout, setLayout] = useState<ShowcaseLayout>(() => normalizeShowcaseLayout(me.showcase_layout));
  const [layoutAnnouncement, setLayoutAnnouncement] = useState('');

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
  const catalog = useQuery({queryKey: ['achievements', 'catalog'], queryFn: getAchievementCatalog});
  // Full-state summary (#79): the featured-achievement picker used to build
  // its counts from the paginated endpoint (cursor never followed), so a key
  // past page one silently couldn't be featured. Same wallet default
  // ('sandbox') as before this fix — only the completeness of the data
  // changed.
  const mine = useAchievementsSummary('sandbox', true);
  const counts = new Map((mine.data?.achievements || []).map(item => [item.key, item.progress]));
  const save = useMutation({
    mutationFn: () => updateMe({
      showcase_public: isPublic, playstyle_public: isPlaystylePublic, table_public: isTablePublic,
      featured_achievements: selected, showcase_layout: layout
    }),
    onSuccess: profile => {
      onSaved(profile);
      pushNotification('Vitrine do perfil atualizada.', 'info');
    },
    onError: () => {
      pushNotification('Não foi possível salvar a vitrine. Tente novamente.', 'error');
    }
  });
  const profileUrl = typeof window === 'undefined' ? `/profile?id=${encodeURIComponent(me.user_id)}` :
    `${window.location.origin}/profile?id=${encodeURIComponent(me.user_id)}`;
  
  const toggle = (key: string, checked: boolean) => {
    if (checked) {
      if (selected.length >= 3) {
        pushNotification('Escolha no máximo três conquistas.', 'info');
        return;
      }
      setSelected(current => [...current, key]);
    } else {
      setSelected(current => current.filter(item => item !== key));
    }
  };
  
  return <>
    <fieldset className="showcase-privacy-group">
      <legend>Perfil público</legend>
      <div className="showcase-privacy-row">
        <span><b>Vitrine pública</b><small>Permite abrir a vitrine por link.</small></span>
        <Switch checked={isPublic} onCheckedChange={setIsPublic} aria-label="Vitrine pública"/>
      </div>
      <div className="showcase-privacy-row">
        <span><b>Estilo de jogo</b><small>Após 200 mãos, exibe um rótulo de tendência na mesa e na vitrine pública.</small></span>
        <Switch checked={isPlaystylePublic} onCheckedChange={setIsPlaystylePublic} disabled={!isPublic}
                aria-label="Estilo de jogo público"/>
      </div>
    </fieldset>
    <fieldset className="showcase-privacy-group">
      <legend>Amigos e mesas</legend>
      <div className="showcase-privacy-row">
        <span><b>Mesa visível para amigos</b><small>Amigos podem entrar na sua mesa quando ela for pública.</small></span>
        <Switch checked={isTablePublic} onCheckedChange={setIsTablePublic} aria-label="Mesa visível para amigos"/>
      </div>
    </fieldset>
    <fieldset className="showcase-achievements">
      <legend>Conquistas em destaque <span>{selected.length}/3</span></legend>
      <p>Somente conquistas com progresso podem ser escolhidas.</p>
      {catalog.isLoading || mine.isLoading ?
        <SkeletonList label="Carregando suas conquistas…" count={3} height={38} className="skeleton-panel"/> :
        catalog.data?.filter(item => (counts.get(item.key) || 0) > 0).map(item => {
          const checked = selected.includes(item.key);
          return <label key={item.key}>
            <Checkbox checked={checked} onCheckedChange={value => toggle(item.key, value === true)}/>
            <span>{achievementLabel(item.key)}<small>{(counts.get(item.key) || 0).toLocaleString('pt-BR')} registrados</small></span>
          </label>;
        })}
    </fieldset>
    <details className="showcase-layout-editor">
      <summary>Personalizar ordem e seções</summary>
      <p>Use as setas para reordenar. Melhor Vitória e Cara a Cara também podem ser escondidos.</p>
      <p className="sr-only" role="status" aria-live="polite">{layoutAnnouncement}</p>
      <ol>
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
    </details>
    {me.showcase_public && <div className="showcase-share-row">
        <Button type="button" variant="outline" onClick={async () => {
          await navigator.clipboard.writeText(profileUrl);
          pushNotification('Link do perfil copiado.', 'info');
        }}><Copy/> Copiar link</Button>
        <Button variant="ghost" render={<Link href={`/profile?id=${encodeURIComponent(me.user_id)}`} target="_blank"/>}>
            Ver perfil <ExternalLink/>
        </Button>
    </div>}
    <DialogFooter>
      <Button type="button" loading={save.isPending} onClick={() => save.mutate()}>
        {save.isPending ? 'Salvando…' : 'Salvar vitrine'}
      </Button>
    </DialogFooter>
  </>;
}

export function ProfileShowcaseDialog({open, onOpenChangeAction}: { open: boolean; onOpenChangeAction: (open: boolean) => void }) {
  const queryClient = useQueryClient();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  return <Dialog open={open} onOpenChange={onOpenChangeAction}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Sua vitrine</DialogTitle>
        <DialogDescription>Escolha o que outros jogadores podem ver. A vitrine começa privada.</DialogDescription>
      </DialogHeader>
      {me && <ShowcaseEditor
          key={`${me.showcase_public}:${me.playstyle_public}:${me.table_public}:${(me.featured_achievements || []).join(',')}:${JSON.stringify(me.showcase_layout || {})}`}
          me={me} onSaved={profile => queryClient.setQueryData(['player', 'me'], profile)}/>}
    </DialogContent>
  </Dialog>;
}
