import Link from 'next/link';
import {ArrowLeft, Club, House, RefreshCw, ShieldAlert, Wrench} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PokerLogo} from '@/components/PokerLogo';

type SystemStateProps = {
  code: '404' | '500' | '503';
  title: string;
  description: string;
  detail: string;
  onRetryAction?: () => void;
  /** Drives the retry button's pending affordance while the check is running. */
  retryPending?: boolean;
};

const icons = {404: Club, 500: ShieldAlert, 503: Wrench};

export function SystemState({code, title, description, detail, onRetryAction, retryPending}: SystemStateProps) {
  const Icon = icons[code];
  return <main className="system-state">
    <nav>
      <Link href="/" className="brand"><span className="brand-mark"><PokerLogo priority/></span>CTech <b>Poker</b></Link>
    </nav>
    <section>
      <div className="system-state-mark" aria-hidden="true">
        <Icon/>
        <span>{code}</span>
      </div>
      <div className="system-state-copy">
        <small>{code === '404' ? 'FORA DA MESA' : code === '500' ? 'A MÃO FOI INTERROMPIDA' : 'INTERVALO TÉCNICO'}</small>
        <h1>{title}</h1>
        <p>{description}</p>
        {/* The detail line is the only part of this screen that changes while
            it is open (a retry's outcome, the countdown to the next one), so
            it is the live region. */}
        <span role="status" aria-live="polite">{detail}</span>
        <div className="system-state-actions">
          {onRetryAction && <Button size="lg" loading={retryPending} onClick={onRetryAction}>
            {!retryPending && <RefreshCw/>} {retryPending ? 'Verificando…' : 'Tentar novamente'}
          </Button>}
          <Button size="lg" variant={onRetryAction ? 'outline' : 'default'} render={<Link href="/"/>}>
            <House/> Ir para o início
          </Button>
          {code === '404' && <Button size="lg" variant="ghost" render={<Link href="/lobby"/>}>
              <ArrowLeft/> Voltar ao lobby
          </Button>}
        </div>
      </div>
    </section>
    <footer>Suas fichas e seu histórico permanecem seguros.</footer>
  </main>;
}
