'use client';
import {useState} from 'react';
import {FlaskConical} from 'lucide-react';
import {useRouter, useSearchParams} from 'next/navigation';
import type {MockScenario} from '@/lib/mockConfig';

const scenarios: { value: MockScenario; label: string }[] = [
  {value: 'full_hand', label: 'Mesa-bot · vitória'},
  {value: 'heads_up', label: 'Layout · heads-up'},
  {value: 'six_max', label: 'Layout · 6-max'},
  {value: 'nine_max', label: 'Layout · 9-max'},
  {value: 'full_hand_loss', label: 'Mesa-bot · derrota'},
  {value: 'full_hand_tie', label: 'Mesa-bot · empate'},
  {value: 'all_in', label: 'Mesa-bot · rival all-in'},
  {value: 'auto_fold', label: 'Prazo esgotado · auto-fold'},
  {value: 'waiting', label: 'Aguardando jogadores'},
  {value: 'pre_flop', label: 'Pré-flop · sua vez'},
  {value: 'flop', label: 'Flop · mesa livre'},
  {value: 'turn', label: 'Turn · sua vez'},
  {value: 'river', label: 'River · vez do rival'},
  {value: 'showdown', label: 'Showdown · vitória'},
  {value: 'side_pot', label: 'Showdown · pote lateral (2 vencedores)'},
  {value: 'run_it_twice', label: 'Showdown · rodar duas vezes'},
  {value: 'winner_cards', label: 'Resultado · pagar para ver a mão'},
  {value: 'rabbit_hunt', label: 'Resultado · rabbit hunt'},
  {value: 'rebuy', label: 'Saldo zerado · recompra'},
  {value: 'reality_check', label: 'Sessão longa · pausa consciente'},
  {value: 'complete', label: 'Resultado · vitória'},
  {value: 'complete_loss', label: 'Resultado · derrota'},
  {value: 'complete_tie', label: 'Resultado · empate'},
  {value: 'fold_win', label: 'Resultado · todos foldaram'},
  {value: 'reconnecting', label: 'Queda e reconexão'},
  {value: 'action_error', label: 'Ação rejeitada'},
  {value: 'timeout', label: 'Servidor sem resposta'},
];

const MOCK_ERROR_KEY = 'ctech_poker_mock_errors';
const errorOptions = [
  {value: '', label: 'Nenhum'},
  {value: '400', label: '400 · Requisição inválida'},
  {value: '401', label: '401 · Sessão expirada'},
  {value: '403', label: '403 · Acesso negado'},
  {value: '404', label: '404 · Não encontrado'},
  {value: '409', label: '409 · Conflito'},
  {value: '429', label: '429 · Muitas requisições'},
  {value: '500', label: '500 · Erro do servidor'},
  {value: 'network', label: 'Erro de rede'},
];

function readGlobalMockError(): string {
  if (typeof window === 'undefined') return '';
  try {
    const rule = JSON.parse(window.localStorage.getItem(MOCK_ERROR_KEY) || '{}')['* *'];
    if (!rule) return '';
    return rule.status === 0 ? 'network' : String(rule.status);
  } catch {
    return '';
  }
}

/** Every REST call goes through this rule first (see forcedError in dev/mockRuntime.ts).
 * A blunt, global way to check every screen's error handling without per-endpoint plumbing. */
function writeGlobalMockError(value: string) {
  if (!value) {
    window.localStorage.removeItem(MOCK_ERROR_KEY);
    return;
  }
  window.localStorage.setItem(MOCK_ERROR_KEY, JSON.stringify({'* *': {status: value === 'network' ? 0 : Number(value)}}));
}

export function MockControls({scenario, delay}: { scenario?: MockScenario; delay?: number }) {
  const router = useRouter();
  const params = useSearchParams();
  const [errorMode, setErrorMode] = useState(readGlobalMockError);
  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params.toString());
    next.set(key, value);
    router.replace(`?${next.toString()}`, {scroll: false});
  };
  
  return <details className="mock-controls">
    <summary><FlaskConical aria-hidden="true"/><span>Cenários de teste</span></summary>
    <div>
      {scenario && <label>Cena
          <select value={scenario} onChange={event => update('scenario', event.target.value)}>
            {scenarios.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
          </select>
      </label>}
      {delay != null && <label>Latência
          <select value={delay} onChange={event => update('delay', event.target.value)}>
              <option value="0">Sem atraso</option>
              <option value="350">350 ms</option>
              <option value="1200">1,2 s</option>
              <option value="9000">9 s · timeout</option>
          </select>
      </label>}
      <label>Simular erro (toda requisição)
        <select value={errorMode} onChange={event => {
          setErrorMode(event.target.value);
          writeGlobalMockError(event.target.value);
        }}>
          {errorOptions.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
      </label>
      <p>Mock local · Axios + realtime em memória</p>
    </div>
  </details>;
}
