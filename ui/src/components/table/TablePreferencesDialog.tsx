'use client';
import {AudioLines, Keyboard, Lightbulb, LockKeyhole, Mic, Repeat2, Settings2, Volume2} from 'lucide-react';
import {useState} from 'react';
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
import {listCosmeticCatalog, ownedCosmeticIDs} from '@/lib/api/cosmeticPurchases';
import {PREMIUM_FELT_IDS, TABLE_THEMES, type TableThemeId, useTablePreferences} from '@/lib/tablePreferences';

const REALITY_OPTIONS = [
  {value: '0', label: 'Desativado'},
  {value: '30', label: 'A cada 30 minutos'},
  {value: '60', label: 'A cada 1 hora'},
  {value: '90', label: 'A cada 1h30'},
  {value: '120', label: 'A cada 2 horas'}
];

export function TablePreferencesDialog({runItTwiceAvailable = false, runItTwice = false, onRunItTwiceChange,
                                         onLockedFeltAction, open, onOpenChangeAction, showTrigger = true}: {
  runItTwiceAvailable?: boolean;
  runItTwice?: boolean;
  onRunItTwiceChange?: (enabled: boolean) => boolean;
  onLockedFeltAction?: (id: TableThemeId) => void;
  open?: boolean;
  onOpenChangeAction?: (open: boolean) => void;
  showTrigger?: boolean;
}) {
  const {preferences, update} = useTablePreferences();
  const queryClient = useQueryClient();
  const {data: me} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});
  // The felt catalog only feeds the picker inside the dialog, which most
  // players never open — so it is fetched on the first open and then kept in
  // cache for every later one (#232). The dialog is open either because a
  // parent controls it (`open`) or because the trigger opened it, hence both
  // the render-time latch and the one in `onOpenChange`.
  const [opened, setOpened] = useState(false);
  if (open && !opened) setOpened(true);
  const catalog = useQuery({
    queryKey: ['wallet', 'cosmetic-catalog', 'felt'], queryFn: () => listCosmeticCatalog('felt'),
    enabled: opened
  });
  const owned = ownedCosmeticIDs(catalog.data ?? []);
  // Ownership is only a fact once the catalog lands. During that first beat a
  // premium felt must not flash a padlock at a player who owns it (and must
  // not be selectable either) — it uses the Select's own disabled state until
  // the answer is in. A failed request is not "still loading": it falls back
  // to locked, which at least routes to the store.
  const ownershipKnown = !catalog.isLoading;
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

  return <Dialog open={open} onOpenChange={next => {
    if (next) setOpened(true);
    onOpenChangeAction?.(next);
  }}>
    {showTrigger && <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Preferências da mesa"/>}>
      <Settings2/>
    </DialogTrigger>}
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
                const premium = PREMIUM_FELT_IDS.has(id as TableThemeId);
                const locked = premium && ownershipKnown && !owned.has(id as TableThemeId);
                return <SelectItem key={id} value={id as TableThemeId} label={feltTheme.label}
                                   disabled={premium && !ownershipKnown}>
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
          <span><AudioLines aria-hidden="true"/><span><Label id="sound-effects-label">Sons da mesa</Label>
            <small>Cartas, fichas e alertas. Começa sem som até você ativar.</small></span></span>
          <Switch aria-labelledby="sound-effects-label" checked={preferences.soundEffects}
                  onCheckedChange={checked => update({soundEffects: checked})}/>
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
        <div className="table-preference-toggle">
          <span><Lightbulb aria-hidden="true"/><span><Label id="equity-trainer-label">Treinador</Label>
            <small>Explica sua mão após agir, só em mesas sandbox. Nunca aparece durante sua decisão nem em dinheiro real.</small></span></span>
          <Switch aria-labelledby="equity-trainer-label" checked={preferences.equityTrainer}
                  onCheckedChange={checked => update({equityTrainer: checked})}/>
        </div>
        <div className="table-preference-toggle">
          <span><Keyboard aria-hidden="true"/><span><Label id="keyboard-shortcuts-label">Atalhos de teclado</Label>
            <small>F, C, P e R agem na sua vez; X, C e A preparam a próxima jogada. Sem remapeamento — só
              ligar ou desligar. Os botões continuam clicáveis de qualquer jeito.</small></span></span>
          <Switch aria-labelledby="keyboard-shortcuts-label" checked={preferences.keyboardShortcuts}
                  onCheckedChange={checked => update({keyboardShortcuts: checked})}/>
        </div>
        {preferences.keyboardShortcuts && <dl className="table-shortcuts-legend" aria-label="Atalhos ativos">
          <div><dt>F</dt><dd>Fold</dd></div>
          <div><dt>C</dt><dd>Check / Call</dd></div>
          <div><dt>P</dt><dd>Pagar</dd></div>
          <div><dt>R</dt><dd>Aumentar</dd></div>
          <div><dt>X</dt><dd>Preparar Check/Fold</dd></div>
          <div><dt>A</dt><dd>Máximo / All In</dd></div>
          <div><dt>← →</dt><dd>Ajustar valor</dd></div>
        </dl>}
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
