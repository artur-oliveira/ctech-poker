'use client';
import dynamic from 'next/dynamic';
import {useEffect, useState} from 'react';
import {LayoutGrid} from 'lucide-react';
import {StakesGrid} from '@/components/lobby/StakesGrid';
import {ActiveTableBanner} from '@/components/lobby/ActiveTableBanner';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';
import {OnboardingIntro} from '@/components/lobby/OnboardingIntro';
import {TermsGate} from '@/components/TermsGate';
import {USE_MOCK} from '@/lib/mockConfig';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
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
    <AppPage authed rewardReady={rewardReady}>
      <AppPageBody className="lobby">
        <AppPageHeader icon={LayoutGrid} eyebrow="LOBBY SANDBOX"
          title="Escolha os blinds e o tamanho da mesa."
          description="Buscamos uma mesa pública com vaga para sua escolha; se não houver, criamos uma nova. Tudo com fichas sandbox."
          actions={<CreateRoomDialog/>}/>
        <ActiveTableBanner/>
        <OnboardingIntro/>
        <StakesGrid/>
      </AppPageBody>
      {USE_MOCK && <MockControls/>}
    </AppPage>
  </TermsGate>;
}
