'use client';
import type {ReactNode} from 'react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';

export interface ActivityRow {
  id: string;
  label: string;
  /** Payment method plus date, already formatted for display. */
  detail: string;
  status: string;
  statusLabel: string;
  media?: ReactNode;
  /** Sort key: most recent first. */
  at: string;
}

// One receipt list for every non-chip purchase stream. Reactions, decks and
// felts are the same object — a one-time entitlement bought with Pix or fichas
// — so they get one list rather than three near-identical copies (the chip
// packs keep PurchaseHistoryList, which carries the Pix resume/refund actions).
export function PurchaseActivityList({rows, isLoading = false, isError = false, loadingLabel, errorLabel,
                                      emptyLabel, onRetryAction, limit = 4}: {
  rows: ActivityRow[];
  isLoading?: boolean;
  isError?: boolean;
  loadingLabel: string;
  errorLabel: string;
  emptyLabel: string;
  onRetryAction?: () => void;
  limit?: number;
}) {
  if (isLoading) return <SkeletonList label={loadingLabel} count={2} height={58} className="store-activity-list"/>;
  if (isError) return <div className="store-activity-error">{errorLabel}
    {onRetryAction && <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button>}
  </div>;

  const history = [...rows].sort((a, b) => b.at.localeCompare(a.at)).slice(0, limit);
  if (!history.length) return <p className="store-history-empty">{emptyLabel}</p>;

  return <ul className="store-activity-list">{history.map(row =>
    <li key={row.id}>
      <span>{row.media}</span>
      <span><strong>{row.label}</strong><small>{row.detail}</small></span>
      <b className={`store-activity-status ${row.status}`}>{row.statusLabel}</b>
    </li>)}
  </ul>;
}
