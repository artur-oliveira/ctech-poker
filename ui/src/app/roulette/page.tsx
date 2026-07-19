'use client'
import {useState} from 'react';
import Link from 'next/link';
import {ChevronLeft, Sparkles} from 'lucide-react';
import {spin} from '@/lib/api/gamification';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';

export default function Roulette() {
  const [turning, setTurning] = useState(false), [result, setResult] = useState<number | null>(null), [error, setError] = useState('');

  async function go() {
    setTurning(true);
    setError('');
    try {
      const r = await spin();
      setTimeout(() => {
        setResult(r.amount);
        setTurning(false)
      }, 1400)
    } catch {
      setTurning(false);
      setError('Não foi possível girar agora. Tente novamente em instantes.')
    }
  }

  return <TermsGate><main className="app-page">
    <section className="roulette"><Link href="/lobby"><ChevronLeft/> Lobby</Link><small>RECOMPENSA DIÁRIA</small>
      <h1>Roleta Sandbox</h1><p>Ganhe fichas virtuais para continuar jogando.</p>
      <div className={`wheel ${turning ? 'turning' : ''}`}>
        <Sparkles/><span>100</span><span>200</span><span>500</span><span>1000</span></div>
      {result && <strong>+{result} fichas sandbox</strong>}{error && <em>{error}</em>}
      <Button size="lg" disabled={turning} onClick={go}>{turning ? 'Girando…' : 'Girar agora'}</Button>
    </section>
  </main></TermsGate>
}
