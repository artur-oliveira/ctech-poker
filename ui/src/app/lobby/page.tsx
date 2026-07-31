'use client';
import dynamic from 'next/dynamic';
import {useEffect, useState} from 'react';
import {StakesGrid} from '@/components/lobby/StakesGrid';
import {ActiveTableBanner} from '@/components/lobby/ActiveTableBanner';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';
import {OnboardingIntro} from '@/components/lobby/OnboardingIntro';
import {TermsGate} from '@/components/TermsGate';
import {USE_MOCK} from '@/lib/mockConfig';
import {AppPageNav} from '@/components/AppPageChrome';
import {getCooldown} from '@/lib/api/dailyReward';

const MockControls = USE_MOCK
  ? dynamic(() => import('@/components/table/MockControls').then(module => module.MockControls))
  : () => null;

export default function Lobby() {
  const [rewardReady, setRewardReady] = useState(false);

  useEffect(() => {
    let active = true;
    getCooldown()
      .then(result => active && setRewardReady(result.remaining_time_seconds === 0))
      .catch(() => undefined);
    return () => { active = false; };
  }, []);

  return <TermsGate>
    <main className="app-page">
      <AppPageNav authed rewardReady={rewardReady}/>
      <section className="lobby shell">
        <OnboardingIntro/>
        <header>
          <div>
            <small>LOBBY SANDBOX</small>
            <h1>Escolha sua mesa.</h1>
            <p>
              Fichas virtuais, emoção de verdade.
            </p>
          </div>
          <div className="lobby-actions">
            <CreateRoomDialog/>
          </div>
        </header>
        <ActiveTableBanner/>
        <StakesGrid/>
      </section>
      {USE_MOCK && <MockControls/>}
    </main>
  </TermsGate>;
}
