import Link from 'next/link';
import {ErrorState, Button} from '@aoctech/ui';
import {House, RefreshCw} from 'lucide-react';
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

/** Poker's frame (brand, background, footer) around the shared `ErrorState`. */
export function SystemState({code, title, description, detail, onRetryAction, retryPending}: SystemStateProps) {
  return <main className="system-state">
    <nav>
      <Link href="/" className="brand"><span className="brand-mark"><PokerLogo priority/></span>CTech <b>Poker</b></Link>
    </nav>
    <section data-ctech-bridge="">
      <ErrorState
        status={Number(code) as 404 | 500 | 503}
        locale="pt-BR"
        title={title}
        description={description}
        action={<>
          {onRetryAction && <Button size="lg" loading={retryPending} onClick={onRetryAction}>
            {!retryPending && <RefreshCw/>} {retryPending ? 'Verificando…' : 'Tentar novamente'}
          </Button>}
          <Button size="lg" variant={onRetryAction ? 'outline' : 'brand'} render={<Link href="/"/>}>
            <House/> Ir para o início
          </Button>
          {code === '404' && <Button size="lg" variant="ghost" render={<Link href="/lobby"/>}>Voltar ao lobby</Button>}
        </>}
      />
      {/* The only part that changes while the screen is open (a retry's outcome), so it is the live region. */}
      <p className="system-state-detail" role="status" aria-live="polite">{detail}</p>
    </section>
    <footer>Suas fichas e seu histórico permanecem seguros.</footer>
  </main>;
}
