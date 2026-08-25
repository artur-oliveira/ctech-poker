import type {ReactNode} from 'react';
import {CircleAlert} from 'lucide-react';

/**
 * The recovery composition the replay page already used for a link that lost
 * its address: mark, plain-language heading, what happened, one way back.
 * A missing-parameter archive link is a trust moment, not a stray error line,
 * so hand history reuses the same vocabulary instead of a bare `.form-error`.
 *
 * `nested` is for a page that already owns the viewport and paints its own
 * background (hand history renders inside `main.app-page`); a `main` landmark
 * is always supplied by the caller, never nested here.
 */
export function RecoveryState({title, description, action, nested = false}: {
  title: string;
  description: string;
  action: ReactNode;
  nested?: boolean;
}) {
  return <div className={nested ? 'recovery-state is-nested' : 'recovery-state'}>
    <div className="recovery-state-mark" aria-hidden="true"><CircleAlert/></div>
    <h1>{title}</h1>
    <p>{description}</p>
    {action}
  </div>;
}
