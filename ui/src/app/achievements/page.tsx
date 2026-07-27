'use client';
import {useCallback, useMemo, useState} from 'react';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, Club, History, Sparkles, Trophy} from 'lucide-react';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {AchievementCard} from '@/components/achievements/AchievementCard';
import {Button} from '@/components/ui/button';
import {achievementProgress, getAchievementCatalog, getMyAchievements} from '@/lib/api/achievements';
import {useOptionalSession} from "@/lib/auth/session";

type FilterTab = 'all' | 'unlocked' | 'in_progress' | 'completed';

export default function Achievements() {
  const {authed, checking} = useOptionalSession();
  const catalog = useQuery({queryKey: ['achievements', 'catalog'], queryFn: getAchievementCatalog});
  const mine = useQuery({queryKey: ['achievements', 'me'], queryFn: () => getMyAchievements(), enabled: authed});
  const [activeTab, setActiveTab] = useState<FilterTab>('all');

  const progressMap = useMemo(() => new Map((mine.data || []).map(p => [p.key, p.count])), [mine.data]);

  // While mine is still loading, leave count undefined (renders neutral tier ladder)
  const countFor = useCallback((key: string) => authed && !mine.isLoading ? progressMap.get(key) ?? 0 : undefined, [authed, mine.isLoading, progressMap]);

  const stats = useMemo(() => {
    if (!authed || !catalog.data || mine.isLoading) return null;
    let starsEarned = 0;
    let completedCount = 0;
    let unlockedCount = 0;
    const maxStars = catalog.data.length * 5;

    for (const item of catalog.data) {
      const cnt = progressMap.get(item.key) ?? 0;
      const p = achievementProgress(item.tiers, cnt);
      starsEarned += p.starsFilled;
      if (p.starsFilled > 0) unlockedCount++;
      if (p.maxed) completedCount++;
    }

    const completionRate = maxStars > 0 ? Math.round((starsEarned / maxStars) * 100) : 0;

    return {starsEarned, maxStars, completedCount, unlockedCount, completionRate};
  }, [authed, catalog.data, mine.isLoading, progressMap]);

  const filteredCatalog = useMemo(() => {
    if (!catalog.data) return [];
    if (!authed || activeTab === 'all') return catalog.data;

    return catalog.data.filter(item => {
      const cnt = countFor(item.key) ?? 0;
      const p = achievementProgress(item.tiers, cnt);
      if (activeTab === 'completed') return p.maxed;
      if (activeTab === 'in_progress') return p.starsFilled > 0 && !p.maxed;
      if (activeTab === 'unlocked') return p.starsFilled > 0;
      return true;
    });
  }, [catalog.data, authed, activeTab, countFor]);

  if (checking) return <div className="loading-screen"><span className="loader"/>Carregando conquistas…</div>;

  return <main className="app-page">
    <nav className="app-nav shell">
      <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
      {authed ? <div className="header-right">
        <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
        <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
        <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
        <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
        <ProfileMenu/>
      </div> : <Link href="/"><ChevronLeft/> Voltar</Link>}
    </nav>
    <section className="achievements shell">
      {authed && <Link href="/lobby"><ChevronLeft/> Lobby</Link>}
      <header>
        <small>PROGRESSO DO JOGADOR</small>
        <Trophy aria-hidden="true"/>
        <h1>Conquistas</h1>
        <p>{authed
          ? 'Cada estrela representa uma meta vencida. Passe o mouse em uma estrela para conferir o requisito de cada nível.'
          : 'Entre com sua conta CTech para registrar seu progresso e desbloquear conquistas a cada mão.'}</p>
      </header>

      {stats && (
        <div className="achievements-stats-bar">
          <div className="stat-card">
            <span className="stat-label">Estrelas conquistadas</span>
            <strong className="stat-value">{stats.starsEarned} <small>/ {stats.maxStars}</small></strong>
          </div>
          <div className="stat-card">
            <span className="stat-label">Desbloqueadas</span>
            <strong className="stat-value">{stats.unlockedCount} <small>/ {catalog.data?.length ?? 0}</small></strong>
          </div>
          <div className="stat-card">
            <span className="stat-label">Completas</span>
            <strong className="stat-value">{stats.completedCount}</strong>
          </div>
          <div className="stat-card">
            <span className="stat-label">Maestria geral</span>
            <strong className="stat-value">{stats.completionRate}%</strong>
          </div>
        </div>
      )}

      {authed && !mine.isLoading && !mine.isError && catalog.data && (
        <div className="filter-tabs" role="tablist" aria-label="Filtro de conquistas">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'all'}
            className={`filter-tab${activeTab === 'all' ? ' active' : ''}`}
            onClick={() => setActiveTab('all')}
          >
            Todas ({catalog.data.length})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'unlocked'}
            className={`filter-tab${activeTab === 'unlocked' ? ' active' : ''}`}
            onClick={() => setActiveTab('unlocked')}
          >
            Desbloqueadas ({stats?.unlockedCount ?? 0})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'in_progress'}
            className={`filter-tab${activeTab === 'in_progress' ? ' active' : ''}`}
            onClick={() => setActiveTab('in_progress')}
          >
            Em progresso ({(stats?.unlockedCount ?? 0) - (stats?.completedCount ?? 0)})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'completed'}
            className={`filter-tab${activeTab === 'completed' ? ' active' : ''}`}
            onClick={() => setActiveTab('completed')}
          >
            Completas ({stats?.completedCount ?? 0})
          </button>
        </div>
      )}

      {authed && mine.isError &&
          <p className="form-error">Não foi possível carregar seu progresso no momento. O catálogo abaixo exibe as metas gerais.</p>}
      {catalog.isLoading ? <div className="lobby-empty"><span className="loader"/>Carregando catálogo de conquistas…</div>
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
    </section>
  </main>;
}
