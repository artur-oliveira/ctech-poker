'use client';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {BookOpen, ChevronLeft, Club, Trophy} from 'lucide-react';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {AchievementCard} from '@/components/achievements/AchievementCard';
import {Button} from '@/components/ui/button';
import {getAchievementCatalog, getMyAchievements} from '@/lib/api/achievements';
import {useOptionalSession} from "@/lib/auth/session";


export default function Achievements() {
  const {authed, checking} = useOptionalSession();
  const catalog = useQuery({queryKey: ['achievements', 'catalog'], queryFn: getAchievementCatalog});
  const mine = useQuery({queryKey: ['achievements', 'me'], queryFn: getMyAchievements, enabled: authed});
  const progress = new Map((mine.data || []).map(p => [p.key, p.count]));
  // While mine is still loading, leave count undefined (renders the neutral
  // tier ladder) instead of flashing "0" for every card before the real
  // numbers land.
  const countFor = (key: string) => authed && !mine.isLoading ? progress.get(key) ?? 0 : undefined;

  if (checking) return <div className="loading-screen"><span className="loader"/>Carregando…</div>;

  return <main className="app-page">
    <nav className="app-nav shell">
      <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
      {authed ? <div className="header-right">
        <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
        <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
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
          ? 'Cada estrela marca um degrau vencido. Passe o mouse ou o foco em uma estrela para ver a meta dela.'
          : 'Entre com sua conta CTech para acompanhar seu próprio progresso em cada uma.'}</p>
      </header>
      {authed && mine.isError &&
          <p className="form-error">Não foi possível carregar seu progresso agora. As conquistas abaixo mostram só o
              catálogo.</p>}
      {catalog.isLoading ? <div className="lobby-empty"><span className="loader"/>Carregando conquistas…</div>
        : catalog.isError ? <div className="lobby-empty">Não foi possível carregar as conquistas.
            <Button variant="outline" size="sm" onClick={() => void catalog.refetch()}>Tentar novamente</Button>
          </div>
          : <div className="achievements-grid">
            {catalog.data!.map(a => <AchievementCard key={a.key} achievement={a} count={countFor(a.key)}/>)}
          </div>}
    </section>
  </main>;
}
