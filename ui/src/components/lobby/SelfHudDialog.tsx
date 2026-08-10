'use client';

import {useQuery} from '@tanstack/react-query';
import {Activity, Info} from 'lucide-react';
import {PlaystyleBadges} from '@/components/PlaystyleBadges';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {getMyPokerStats, type PokerStats} from '@/lib/api/pokerStats';
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {SkeletonList} from '@/components/ui/skeleton';
import {useState} from 'react';
import type {WalletMode} from '@/lib/api/player';

function percentage(value: number) {
  return `${(value * 100).toLocaleString('pt-BR', {maximumFractionDigits: 1})}%`;
}

const RADAR_AXES = ['Participação', 'Iniciativa', 'Pressão', 'Reaumento', 'Seleção'] as const;

function radarPoint(index: number, value: number, radius = 82) {
  const angle = -Math.PI / 2 + index * Math.PI * 2 / RADAR_AXES.length;
  return [100 + Math.cos(angle) * radius * value, 100 + Math.sin(angle) * radius * value];
}

function points(values: number[], radius = 82) {
  return values.map((value, index) => radarPoint(index, value, radius).join(',')).join(' ');
}

function PokerStyle({stats}: { stats: PokerStats }) {
  const pressure = stats.vpip_rate > 0 ? stats.pfr_rate / stats.vpip_rate : 0;
  const values = [
    Math.min(1, stats.vpip_rate / .5),
    Math.min(1, stats.pfr_rate / .35),
    Math.min(1, pressure),
    Math.min(1, stats.three_bet_rate / .18),
    Math.max(0, 1 - Math.min(1, stats.vpip_rate / .5))
  ];
  return <section className="poker-style" aria-labelledby="poker-style-title">
    <div className="poker-style-copy">
      <small>Leitura da amostra</small>
      <h3 id="poker-style-title">Seu estilo pré-flop</h3>
      <PlaystyleBadges badges={stats.playstyle || []}/>
      <p>Os eixos são derivados de VPIP, PFR e 3-bet. Abra um badge para conferir o critério.</p>
    </div>
    <div className="poker-style-radar">
      <svg viewBox="0 0 200 200" role="img"
           aria-label={`Radar de estilo: ${RADAR_AXES.map((axis, index) => `${axis} ${Math.round(values[index] * 100)}%`).join(', ')}`}>
        {[.25, .5, .75, 1].map(level =>
          <polygon key={level} points={points(RADAR_AXES.map(() => level))} className="poker-radar-grid"/>)}
        {RADAR_AXES.map((axis, index) => {
          const [x, y] = radarPoint(index, 1);
          const [labelX, labelY] = radarPoint(index, 1.16);
          return <g key={axis}>
            <line x1="100" y1="100" x2={x} y2={y} className="poker-radar-axis"/>
            <text x={labelX} y={labelY}
                  textAnchor={labelX < 94 ? 'end' : labelX > 106 ? 'start' : 'middle'}>{axis}</text>
          </g>;
        })}
        <polygon points={points(values)} className="poker-radar-value"/>
        {values.map((value, index) => {
          const [x, y] = radarPoint(index, value);
          return <circle key={RADAR_AXES[index]} cx={x} cy={y} r="3"/>;
        })}
      </svg>
    </div>
  </section>;
}

const METRICS: Array<{
  key: 'vpip_rate' | 'pfr_rate' | 'three_bet_rate';
  label: string;
  short: string;
  description: string;
  sample: (stats: PokerStats) => number;
}> = [{
  key: 'vpip_rate',
  label: 'VPIP',
  short: 'Entrou voluntariamente',
  description: 'Mãos em que você colocou fichas no pote pré-flop, sem contar os blinds.',
  sample: stats => stats.hands
}, {
  key: 'pfr_rate',
  label: 'PFR',
  short: 'Aumentou pré-flop',
  description: 'Mãos em que você fez pelo menos um raise pré-flop.',
  sample: stats => stats.hands
}, {
  key: 'three_bet_rate',
  label: '3-bet',
  short: 'Reaumentou',
  description: 'Vezes em que você reaumentou diante de um raise, entre as oportunidades reais.',
  sample: stats => stats.three_bet_chances
}];

function HudContent({stats}: { stats: PokerStats }) {
  if (stats.hands === 0) {
    return <div className="self-hud-empty">
      <Activity aria-hidden="true"/>
      <b>Suas tendências aparecem depois da primeira mão.</b>
      <span>Somente você pode ver estes dados.</span>
    </div>;
  }
  return <>
    <div className="self-hud-sample">
      <span>Amostra</span>
      <b>{stats.hands.toLocaleString('pt-BR')} {stats.hands === 1 ? 'mão' : 'mãos'}</b>
    </div>
    <div className="self-hud-metrics">
      {METRICS.map(metric => <article key={metric.key}>
        <header><span>{metric.label}</span><strong>{percentage(stats[metric.key])}</strong></header>
        <b>{metric.short}</b>
        <p>{metric.description}</p>
        <small>Base: {metric.sample(stats).toLocaleString('pt-BR')}</small>
      </article>)}
    </div>
    {Boolean(stats.playstyle?.length) && <PokerStyle stats={stats}/>}
    {!stats.playstyle?.length && <p className="self-hud-notice"><Info aria-hidden="true"/>
        Amostra inicial: as porcentagens ficam mais representativas conforme você joga.
    </p>}
  </>;
}

export function SelfHudDialog({open, onOpenChangeAction}: { open: boolean; onOpenChangeAction: (open: boolean) => void }) {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const query = useQuery({
    queryKey: ['poker-stats', 'me', mode],
    queryFn: () => getMyPokerStats(mode),
    enabled: open
  });
  return <Dialog open={open} onOpenChangeAction={onOpenChangeAction}>
    <DialogContent className="self-hud-dialog">
      <DialogHeader>
        <DialogTitle>Seu jogo</DialogTitle>
        <DialogDescription>Estatísticas privadas calculadas a partir das suas mãos concluídas.</DialogDescription>
      </DialogHeader>
      <CurrencyModeTabs mode={mode} onChangeAction={setMode}/>
      {query.isLoading ?
        <SkeletonList label="Calculando tendências…" count={4} height={52} className="skeleton-panel"/> :
        query.data ? <HudContent stats={query.data}/> :
          <div className="self-hud-empty"><b>Não foi possível carregar
            agora.</b><span>Tente novamente em instantes.</span></div>}
    </DialogContent>
  </Dialog>;
}
