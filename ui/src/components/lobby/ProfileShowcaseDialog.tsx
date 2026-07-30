'use client';
import {useState} from 'react';
import Link from 'next/link';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Copy, ExternalLink} from 'lucide-react';
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
import {getAchievementCatalog, getMyAchievements} from '@/lib/api/achievements';
import {getMe, type PlayerProfile, updateMe} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';
import {pushNotification} from '@/lib/notify';
import {SkeletonList} from '@/components/ui/skeleton';

function ShowcaseEditor({me, onSaved}: { me: PlayerProfile; onSaved: (profile: PlayerProfile) => void }) {
  const [isPublic, setIsPublic] = useState(me.showcase_public);
  const [isPlaystylePublic, setIsPlaystylePublic] = useState(me.playstyle_public);
  const [selected, setSelected] = useState<string[]>(me.featured_achievements || []);
  const catalog = useQuery({queryKey: ['achievements', 'catalog'], queryFn: getAchievementCatalog});
  const mine = useQuery({queryKey: ['achievements', 'me'], queryFn: () => getMyAchievements()});
  const counts = new Map((mine.data || []).map(item => [item.key, item.count]));
  const save = useMutation({
    mutationFn: () => updateMe({
      showcase_public: isPublic, playstyle_public: isPlaystylePublic,
      featured_achievements: selected
    }),
    onSuccess: profile => {
      onSaved(profile);
      pushNotification('Vitrine do perfil atualizada.', 'info');
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
    <div className="showcase-privacy-row">
      <span><b>Perfil público</b><small>Permite abrir a vitrine por link.</small></span>
      <Switch checked={isPublic} onCheckedChange={setIsPublic} aria-label="Perfil público"/>
    </div>
    <div className="showcase-privacy-row showcase-playstyle-row">
      <span><b>Estilo de jogo público</b><small>Após 200 mãos, exibe um rótulo de tendência na mesa e na vitrine. Em uma vitrine pública, essa informação pode ser vista sem login.</small></span>
      <Switch checked={isPlaystylePublic} onCheckedChange={setIsPlaystylePublic}
              aria-label="Estilo de jogo público"/>
    </div>
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
    {isPublic && <div className="showcase-share-row">
        <Button type="button" variant="outline" onClick={async () => {
          await navigator.clipboard.writeText(profileUrl);
          pushNotification('Link do perfil copiado.', 'info');
        }}><Copy/> Copiar link</Button>
        <Button variant="ghost" render={<Link href={`/profile?id=${encodeURIComponent(me.user_id)}`} target="_blank"/>}>
            Ver perfil <ExternalLink/>
        </Button>
    </div>}
    <DialogFooter>
      <Button type="button" disabled={save.isPending} onClick={() => save.mutate()}>Salvar vitrine</Button>
    </DialogFooter>
  </>;
}

export function ProfileShowcaseDialog({open, onOpenChange}: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Sua vitrine</DialogTitle>
        <DialogDescription>Escolha o que outros jogadores podem ver. A vitrine começa privada.</DialogDescription>
      </DialogHeader>
      {me && <ShowcaseEditor
          key={`${me.showcase_public}:${me.playstyle_public}:${(me.featured_achievements || []).join(',')}`}
          me={me} onSaved={profile => queryClient.setQueryData(['player', 'me'], profile)}/>}
    </DialogContent>
  </Dialog>;
}
