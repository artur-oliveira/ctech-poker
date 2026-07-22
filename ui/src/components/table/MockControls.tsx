'use client';
import {FlaskConical} from 'lucide-react';
import {useRouter, useSearchParams} from 'next/navigation';
import type {MockScenario} from '@/lib/mock';

const scenarios: { value: MockScenario; label: string }[] = [
  {value: 'full_hand', label: 'Mão completa interativa'},
  {value: 'waiting', label: 'Aguardando jogadores'},
  {value: 'pre_flop', label: 'Pré-flop · sua vez'},
  {value: 'flop', label: 'Flop · mesa livre'},
  {value: 'turn', label: 'Turn · sua vez'},
  {value: 'river', label: 'River · vez do rival'},
  {value: 'showdown', label: 'Showdown · vitória'},
  {value: 'reconnecting', label: 'Queda e reconexão'},
  {value: 'action_error', label: 'Ação rejeitada'},
  {value: 'timeout', label: 'Ação sem resposta'},
];

export function MockControls({scenario, delay}: { scenario: MockScenario; delay: number }) {
  const router = useRouter();
  const params = useSearchParams();
  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params.toString());
    next.set(key, value);
    router.replace(`?${next.toString()}`, {scroll: false});
  };
  
  return <details className="mock-controls">
    <summary><FlaskConical aria-hidden="true"/><span>Cenários de teste</span></summary>
    <div>
      <label>Cena
        <select value={scenario} onChange={event => update('scenario', event.target.value)}>
          {scenarios.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      <label>Latência
        <select value={delay} onChange={event => update('delay', event.target.value)}>
          <option value="0">Sem atraso</option>
          <option value="350">350 ms</option>
          <option value="1200">1,2 s</option>
          <option value="9000">9 s · timeout</option>
        </select>
      </label>
      <p>Mock local · Axios + realtime em memória</p>
    </div>
  </details>;
}
