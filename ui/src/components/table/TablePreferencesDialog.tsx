'use client';
import {LockKeyhole, Mic, Repeat2, Settings2, Volume2} from 'lucide-react';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {Switch} from '@/components/ui/switch';
import {getMe, updateMe} from '@/lib/api/player';
import {listCosmeticCatalog, listCosmeticPurchases, ownedCosmeticIDs} from '@/lib/api/cosmeticPurchases';
import {PREMIUM_FELT_IDS, TABLE_THEMES, type TableThemeId, useTablePreferences} from '@/lib/tablePreferences';

const REALITY_OPTIONS = [
  {value: '0', label: 'Desativado'},
  {value: '30', label: 'A cada 30 minutos'},
  {value: '60', label: 'A cada 1 hora'},
  {value: '90', label: 'A cada 1h30'},
  {value: '120', label: 'A cada 2 horas'}
];

export function TablePreferencesDialog({runItTwiceAvailable = false, runItTwice = false, onRunItTwiceChange,
                                         onLockedFeltAction}: {
  runItTwiceAvailable?: boolean;
  runItTwice?: boolean;
  onRunItTwiceChange?: (enabled: boolean) => boolean;
  onLockedFeltAction?: (id: TableThemeId) => void;
}) {
  const {preferences, update} = useTablePreferences();
  const queryClient = useQueryClient();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  const catalog = useQuery({queryKey: ['wallet', 'cosmetic-catalog', 'felt'], queryFn: () => listCosmeticCatalog('felt')});
  const purchases = useQuery({queryKey: ['wallet', 'cosmetic-purchases', 'felt'], queryFn: () => listCosmeticPurchases('felt')});
  const owned = ownedCosmeticIDs(purchases.data ?? []);
  const prices = new Map((catalog.data ?? []).map(entry => [entry.id, entry.price_fichas]));
  const save = useMutation({
    mutationFn: updateMe,
    onSuccess: data => queryClient.setQueryData(['player', 'me'], data),
  });
  const theme: TableThemeId = me?.table_theme || 'classic';

  function chooseTheme(value: TableThemeId | null) {
    if (!value) return;
    if (PREMIUM_FELT_IDS.has(value) && !owned.has(value)) {
      onLockedFeltAction?.(value);
      return;
    }
    save.mutate({table_theme: value});
  }

  return <Dialog>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Preferências da mesa"/>}>
      <Settings2/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Preferências da mesa</DialogTitle>
        <DialogDescription>Personalize a experiência e escolha como prefere jogar nesta mesa.</DialogDescription>
      </DialogHeader>
      <div className="table-preferences">
        <div>
          <Label id="table-theme-label">Tema do feltro</Label>
          <Select value={theme} onValueChange={chooseTheme}>
            <SelectTrigger aria-labelledby="table-theme-label" disabled={save.isPending}>
              <SelectValue>
                {(value: TableThemeId) => TABLE_THEMES[value]?.label ?? TABLE_THEMES.classic.label}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {Object.entries(TABLE_THEMES).map(([id, feltTheme]) => {
                const locked = PREMIUM_FELT_IDS.has(id as TableThemeId) && !owned.has(id as TableThemeId);
                return <SelectItem key={id} value={id as TableThemeId} label={feltTheme.label}>
                  <span className={`table-theme-option${locked ? ' locked' : ''}`}>
                    <span aria-hidden="true"
                          style={{'--theme-a': feltTheme.colors[0], '--theme-b': feltTheme.colors[1]} as React.CSSProperties}/>
                    {feltTheme.label}
                    {locked && <LockKeyhole aria-label={`Premium bloqueado${
                      prices.get(id) ? ` · ${prices.get(id)!.toLocaleString('pt-BR')} fichas` : ''}`}/>}
                  </span>
                </SelectItem>;
              })}
            </SelectContent>
          </Select>
        </div>
        <div className="table-preference-toggle">
          <span><Volume2 aria-hidden="true"/><span><Label id="dealer-voice-label">Dealer auditivo</Label>
            <small>Narra as principais ações e cartas.</small></span></span>
          <Switch aria-labelledby="dealer-voice-label" checked={preferences.dealerVoice}
                  onCheckedChange={checked => update({dealerVoice: checked})}/>
        </div>
        {runItTwiceAvailable && <div className="table-preference-toggle table-preference-gameplay">
          <span><Repeat2 aria-hidden="true"/><span><Label id="run-it-twice-label">Rodar duas vezes</Label>
            <small>Em all-ins, divide cada pote entre dois boards. Só acontece quando todos os jogadores envolvidos também ativaram.</small>
          </span></span>
            <Switch aria-labelledby="run-it-twice-label" checked={runItTwice}
                    onCheckedChange={checked => onRunItTwiceChange?.(checked)}/>
        </div>}
        <div className="table-preference-toggle">
          <span><Mic aria-hidden="true"/><span><Label id="voice-actions-label">Comandos por voz</Label>
            <small>Push-to-talk. O jogo recebe somente a ação reconhecida, nunca o áudio.</small></span></span>
          <Switch aria-labelledby="voice-actions-label" checked={preferences.voiceCommands}
                  onCheckedChange={checked => update({voiceCommands: checked})}/>
        </div>
        <div>
          <Label id="reality-check-label">Lembrete de sessão</Label>
          <Select value={String(preferences.realityCheckMinutes)}
                  onValueChange={value => value !== null && update({realityCheckMinutes: Number(value)})}>
            <SelectTrigger aria-labelledby="reality-check-label">
              <SelectValue>
                {(value: string) => REALITY_OPTIONS.find(option => option.value === value)?.label}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {REALITY_OPTIONS.map(option =>
                <SelectItem key={option.value} value={option.value} label={option.label}>{option.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>
    </DialogContent>
  </Dialog>;
}
