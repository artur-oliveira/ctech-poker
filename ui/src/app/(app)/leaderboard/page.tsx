'use client';
import React, {useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {Crown, Sparkles} from 'lucide-react';
import {leaderboard, myRank} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';
import {useOptionalSession} from "@/lib/auth/session";
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {SkeletonList} from '@/components/ui/skeleton';
import type {WalletMode} from '@/lib/api/player';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';

export default function Ranking() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const {data = [], isLoading, isError, refetch} = useQuery({
    queryKey: ['leaderboard', mode],
    queryFn: () => leaderboard(mode)
  });
  const viewer = getViewerId();
  const {authed} = useOptionalSession();
  const {data: rankInfo} = useQuery({
    queryKey: ['leaderboard-me', mode],
    queryFn: () => myRank(mode),
    enabled: authed
  });

  const topThree = data.slice(0, 3);
  const remaining = data.slice(3);

  return (
    <AppPage authed={authed} current="leaderboard">
      <AppPageBody className="ranking">
        <AppPageHeader
          icon={Crown}
          eyebrow="HALL DA FAMA"
          title="Ranking da comunidade"
          description="Desempenho auditável baseado em vitórias e mãos jogadas nas mesas do CTech Poker."
        />
        <CurrencyModeTabs mode={mode} onChangeAction={setMode}/>
        
        {authed && rankInfo && (
          <div className="viewer-ranking-card" aria-label="Sua posição atual">
            <Sparkles aria-hidden="true"/>
            {rankInfo.ranked && rankInfo.entry ? (
              <>
                <div>
                  <span>Sua posição no ranking</span>
                  <strong>#{rankInfo.rank} de {rankInfo.total} jogador{rankInfo.total === 1 ? '' : 'es'}</strong>
                </div>
                <div className="viewer-ranking-stats">
                  <span><b>{rankInfo.entry.hands_won}</b> vitórias</span>
                  <span><b>{(rankInfo.entry.win_rate * 100).toFixed(1)}%</b> de taxa de vitória</span>
                </div>
              </>
            ) : (
              <div>
                <span>Sua posição no ranking</span>
                <strong>Ainda sem ranking</strong>
                <p className="viewer-ranking-hint">Jogue uma mão nesta modalidade para entrar no ranking.</p>
              </div>
            )}
          </div>
        )}
        
        {isLoading ?
          <SkeletonList label="Buscando o ranking da comunidade…" count={6} height={62}
                        className="ranking-list skeleton-panel"/> :
          isError ? <div className="lobby-empty">Não foi possível carregar o ranking agora.
              <button type="button" className="link-retry" onClick={() => void refetch()}>Tentar novamente</button>
            </div> :
            !data.length ?
              <div className="lobby-empty">Nenhum jogador pontuou ainda. A primeira mesa inicia o Hall da Fama.</div> :
              <>
                {topThree.length >= 3 && (
                  <div className="leaderboard-podium" aria-label="Pódio do ranking">
                    {topThree.map((player, index) => {
                      const rank = index + 1;
                      const isViewer = player.player_id === viewer;
                      return (
                        <article key={player.player_id}
                                 className={`podium-card rank-${rank}${isViewer ? ' viewer' : ''}`}>
                          {rank === 1 && <Crown className="podium-crown" aria-hidden="true"/>}
                          <span className="podium-badge">{rank}º Lugar</span>
                          <strong className="podium-name">
                            {playerName(player.player_id, viewer, player.player_name)}
                            {isViewer && ' (Você)'}
                          </strong>
                          <div className="podium-stats">
                            <span><strong>{player.hands_won}</strong> vitórias</span>
                            <span>{player.hands_played} mãos ({(player.win_rate * 100).toFixed(1)}%)</span>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                )}
                
                <div className="ranking-list">
                  {(topThree.length >= 3 ? remaining : data).map((e, i) => {
                    const rankNumber = topThree.length >= 3 ? i + 4 : i + 1;
                    const isViewer = e.player_id === viewer;
                    return (
                      <article key={e.player_id}
                               className={isViewer ? 'viewer' : undefined}
                               style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
                        <b>{String(rankNumber).padStart(2, '0')}</b>
                        <span>{playerName(e.player_id, viewer, e.player_name)}{isViewer && ' (Você)'}<small>{e.hands_played} mãos jogadas</small></span>
                        <strong>{e.hands_won} vitórias<small>{(e.win_rate * 100).toFixed(1)}% de aproveitamento</small></strong>
                      </article>
                    );
                  })}
                </div>
              </>}
      </AppPageBody>
    </AppPage>
  );
}
