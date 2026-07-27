'use client';
import {Settings2, Volume2} from 'lucide-react';
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
import {
  TABLE_THEMES,
  type TableThemeId,
  useTablePreferences
} from '@/lib/tablePreferences';

const REALITY_OPTIONS = [
  {value: '0', label: 'Desativado'},
  {value: '30', label: 'A cada 30 minutos'},
  {value: '60', label: 'A cada 1 hora'},
  {value: '90', label: 'A cada 1h30'},
  {value: '120', label: 'A cada 2 horas'}
];

export function TablePreferencesDialog() {
  const {preferences, update} = useTablePreferences();
  return <Dialog>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Preferências da mesa"/>}>
      <Settings2/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Preferências da mesa</DialogTitle>
        <DialogDescription>Ajustes salvos somente neste navegador.</DialogDescription>
      </DialogHeader>
      <div className="table-preferences">
        <div>
          <Label id="table-theme-label">Tema do feltro</Label>
          <Select value={preferences.theme}
                  onValueChange={(value: TableThemeId | null) => value && update({theme: value})}>
            <SelectTrigger aria-labelledby="table-theme-label">
              <SelectValue>
                {(value: TableThemeId) => TABLE_THEMES[value]?.label ?? TABLE_THEMES.classic.label}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {Object.entries(TABLE_THEMES).map(([id, theme]) =>
                <SelectItem key={id} value={id as TableThemeId} label={theme.label}>
                  <span className="table-theme-option">
                    <span aria-hidden="true" style={{'--theme-a': theme.colors[0], '--theme-b': theme.colors[1]} as React.CSSProperties}/>
                    {theme.label}
                  </span>
                </SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="table-preference-toggle">
          <span><Volume2 aria-hidden="true"/><span><Label id="dealer-voice-label">Dealer auditivo</Label>
            <small>Narra as principais ações e cartas.</small></span></span>
          <Switch aria-labelledby="dealer-voice-label" checked={preferences.dealerVoice}
                  onCheckedChange={checked => update({dealerVoice: checked})}/>
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
