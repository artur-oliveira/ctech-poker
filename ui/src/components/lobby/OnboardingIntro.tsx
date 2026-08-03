'use client';
import {useState} from 'react';
import {Compass, X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {hasSeenOnboarding, markOnboardingSeen} from '@/lib/onboarding';

export function OnboardingIntro() {
  const [dismissed, setDismissed] = useState(() => hasSeenOnboarding());
  if (dismissed) return null;
  
  function dismiss() {
    markOnboardingSeen();
    setDismissed(true);
  }
  
  return <div className="onboarding-intro" role="note">
    <div className="onboarding-intro-icon" aria-hidden="true"><Compass/></div>
    <div className="onboarding-intro-copy">
      <b>Sua primeira mesa começa aqui.</b>
      <p>Escolha os blinds e o formato abaixo. Você joga com fichas sandbox e pode entrar sem tutorial.</p>
    </div>
    <Button type="button" variant="ghost" size="icon" aria-label="Fechar introdução"
            className="onboarding-intro-close" onClick={dismiss}><X/></Button>
  </div>;
}
