'use client';
import {type CSSProperties, useCallback, useMemo, useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Sparkles, Star, Trophy} from 'lucide-react';
import {AchievementCard} from '@/components/achievements/AchievementCard';
import {Button} from '@/components/ui/button';
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {FilterGroup} from '@/components/FilterGroup';
import {SkeletonList, StatCardsSkeleton} from '@/components/ui/skeleton';
import {achievementProgress, getAchievementCatalog} from '@/lib/api/achievements';
import {achievementLabel, achievementValueFormat, achievementWalletMode} from '@/lib/achievements';
import type {WalletMode} from '@/lib/api/player';
import {useOptionalSession} from "@/lib/auth/session";
import {useAchievementsSummary} from '@/lib/hooks/useAchievementsSummary';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';

type FilterTab = 'all' | 'unlocked' | 'in_progress' | 'completed';

export default function Achievements() {
  const {authed, checking} = useOptionalSession();
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const catalog = useQuery({queryKey: ['achievements', 'catalog'], queryFn: getAchievementCatalog});
  const mine = useAchievementsSummary(mode, authed);
  const [activeTab, setActiveTab] = useState<FilterTab>('all');

  // Full-state summary (#79): every key the player has touched, not just the
  // first page, so completion %, star totals and secret reveals below are
  // never understated by a followed-but-truncated cursor.
  const progressMap = useMemo(() => new Map((mine.data?.achievements || []).map(p => [p.key, p.progress])), [mine.data]);
  const visibleCatalog = useMemo(() => (catalog.data || []).filter(item => {
    const walletMode = achievementWalletMode(item.key);
    if (walletMode && walletMode !== mode) return false;
    if (!item.secret) return true;
    const firstTier = Math.min(...item.tiers.map(tier => tier.threshold));
    return authed && !mine.isLoading && (progressMap.get(item.key) ?? 0) >= firstTier;
  }), [authed, catalog.data, mine.isLoading, mode, progressMap]);
  
  // While mine is still loading, leave count undefined (renders neutral tier ladder)
  const countFor = useCallback((key: string) => authed && !mine.isLoading ? progressMap.get(key) ?? 0 : undefined, [authed, mine.isLoading, progressMap]);
  
  const stats = useMemo(() => {
    if (!authed || !catalog.data || mine.isLoading) return null;
    let starsEarned = 0;
    let completedCount = 0;
    let unlockedCount = 0;
    const maxStars = visibleCatalog.reduce((sum, item) => sum + Math.max(...item.tiers.map(tier => tier.stars)), 0);
    
    for (const item of visibleCatalog) {
      const cnt = progressMap.get(item.key) ?? 0;
      const p = achievementProgress(item.tiers, cnt);
      starsEarned += p.starsFilled;
      if (p.starsFilled > 0) unlockedCount++;
      if (p.maxed) completedCount++;
    }
    
    const completionRate = maxStars > 0 ? Math.round((starsEarned / maxStars) * 100) : 0;
    
    return {starsEarned, maxStars, completedCount, unlockedCount, completionRate};
  }, [authed, catalog.data, mine.isLoading, progressMap, visibleCatalog]);

  const nextMilestone = useMemo(() => {
    if (!authed || mine.isLoading) return null;
    const candidates = visibleCatalog.flatMap(item => {
      const progress = achievementProgress(item.tiers, progressMap.get(item.key) ?? 0);
      if (!progress.nextTier) return [];
      const previousThreshold = Math.max(0, ...item.tiers
        .filter(tier => tier.threshold <= progress.count)
        .map(tier => tier.threshold));
      const span = progress.nextTier.threshold - previousThreshold;
      return [{item, progress, closeness: span > 0 ? (progress.count - previousThreshold) / span : 0}];
    });
    return candidates.sort((a, b) => b.closeness - a.closeness)[0] ?? null;
  }, [authed, mine.isLoading, progressMap, visibleCatalog]);
  
  const filteredCatalog = useMemo(() => {
    if (!catalog.data) return [];
    if (!authed || activeTab === 'all') return visibleCatalog;
    
    return visibleCatalog.filter(item => {
      const cnt = countFor(item.key) ?? 0;
      const p = achievementProgress(item.tiers, cnt);
      if (activeTab === 'completed') return p.maxed;
      if (activeTab === 'in_progress') return p.starsFilled > 0 && !p.maxed;
      if (activeTab === 'unlocked') return p.starsFilled > 0;
      return true;
    });
  }, [catalog.data, visibleCatalog, authed, activeTab, countFor]);
  
  if (checking) return <div className="loading-screen"><span className="loader"/>Carregando conquistas…</div>;
  
  return <AppPage authed={authed} current="achievements">
    <AppPageBody className="achievements">
      <AppPageHeader
        icon={Trophy}
        eyebrow="PROGRESSO DO JOGADOR"
        title="Conquistas"
        description={authed
          ? 'Cada estrela representa uma meta vencida. Passe, toque ou use o teclado para conferir o requisito de cada nível.'
          : 'Entre com sua conta CTech para registrar seu progresso e desbloquear conquistas a cada mão.'}
      />
      {/* The wallet tabs already say which wallet is selected; a second sentence
          repeating it sat off the page's centred axis and told the player nothing
          they had not just chosen. */}
      {authed && <CurrencyModeTabs mode={mode} onChangeAction={setMode}/>}
      
      {authed && !stats && !catalog.isError && !mine.isError
        ? <StatCardsSkeleton label="Calculando seu progresso…" count={4}/>
        : stats && (
        <section className="achievement-overview" aria-label="Resumo das conquistas">
          <div className="achievement-overview-main">
            <div className="achievement-overview-heading">
              <div>
                <span>Maestria geral</span>
                <strong>{stats.completionRate}%</strong>
              </div>
              <span>{stats.starsEarned} de {stats.maxStars} estrelas</span>
            </div>
            <div className="achievement-overview-track" role="progressbar" aria-label="Maestria geral"
                 aria-valuemin={0} aria-valuemax={100} aria-valuenow={stats.completionRate}>
              <span style={{'--fill': stats.completionRate / 100} as CSSProperties}/>
            </div>
            <div className="achievements-stats-bar">
              <div className="stat-card">
                <span className="stat-label">Estrelas conquistadas</span>
                <strong className="stat-value">{stats.starsEarned} <small>/ {stats.maxStars}</small></strong>
              </div>
              <div className="stat-card">
                <span className="stat-label">Desbloqueadas</span>
                <strong className="stat-value">{stats.unlockedCount} <small>/ {visibleCatalog.length}</small></strong>
              </div>
              <div className="stat-card">
                <span className="stat-label">Completas</span>
                <strong className="stat-value">{stats.completedCount}</strong>
              </div>
            </div>
          </div>
          {nextMilestone && <div className="achievement-next-star">
            <span className="achievement-next-star-icon"><Star fill="currentColor" aria-hidden="true"/></span>
            <div>
              <span>Sua próxima estrela</span>
              <strong>{achievementLabel(nextMilestone.item.key)}</strong>
              <small>Faltam {achievementValueFormat(nextMilestone.item.key)(nextMilestone.progress.nextTier!.threshold - nextMilestone.progress.count)} para o nível {nextMilestone.progress.nextTier!.stars}.</small>
            </div>
          </div>}
        </section>
      )}
      
      {authed && !mine.isLoading && !mine.isError && catalog.data && (
        <FilterGroup
          label="Filtro de conquistas"
          value={activeTab}
          options={[
            {value: 'all', label: `Todas (${visibleCatalog.length})`},
            {value: 'unlocked', label: `Desbloqueadas (${stats?.unlockedCount ?? 0})`},
            {
              value: 'in_progress',
              label: `Em progresso (${(stats?.unlockedCount ?? 0) - (stats?.completedCount ?? 0)})`
            },
            {value: 'completed', label: `Completas (${stats?.completedCount ?? 0})`}
          ]}
          onChangeAction={setActiveTab}
        />
      )}
      
      {authed && mine.isError &&
          <p className="form-error">Não foi possível carregar seu progresso no momento. O catálogo abaixo exibe as metas
              gerais.</p>}
      {/* The cards below are h3s. Without this the page jumped H1 -> H3 and the
          grid had no name of its own in the heading tree; the filters above it
          are the visible label, so the heading itself stays screen-reader-only. */}
      <h2 className="sr-only">Catálogo de conquistas</h2>
      {catalog.isLoading ?
        <SkeletonList label="Carregando catálogo de conquistas…" count={6} height={218}
                      className="achievements-grid"/>
        : catalog.isError ? <div className="lobby-empty">Não foi possível carregar o catálogo de conquistas.
            <Button variant="outline" size="sm" onClick={() => void catalog.refetch()}>Tentar novamente</Button>
          </div>
          : filteredCatalog.length === 0 ? (
            <div className="lobby-empty">
              <Sparkles aria-hidden="true"/>
              <p>Nenhuma conquista nesta categoria.</p>
              <Button variant="outline" size="sm" onClick={() => setActiveTab('all')}>Ver todas</Button>
            </div>
          ) : (
            <div className="achievements-grid">
              {filteredCatalog.map(a => <AchievementCard key={a.key} achievement={a} count={countFor(a.key)}/>)}
            </div>
          )}
    </AppPageBody>
  </AppPage>;
}
