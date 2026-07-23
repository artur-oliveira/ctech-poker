'use client';
import {useState} from 'react';
import Link from 'next/link';
import {BookOpen, Compass, X} from 'lucide-react';
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
      <b>Primeira vez em uma mesa CTech Poker?</b>
      <p>Duas leituras rápidas antes de sentar: as regras do Texas Hold&apos;em e como funciona esta mesa.</p>
    </div>
    <div className="onboarding-intro-actions">
      <Button type="button" variant="outline" render={<Link href="/poker-rules"/>} onClick={dismiss}>
        <BookOpen/> Regras do poker
      </Button>
      <Button type="button" variant="ghost" render={<Link href="/guide"/>} onClick={dismiss}>Como funciona a
        mesa</Button>
    </div>
    <Button type="button" variant="ghost" size="icon" aria-label="Fechar introdução"
      className="onboarding-intro-close" onClick={dismiss}><X/></Button>
  </div>;
}
