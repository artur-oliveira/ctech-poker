'use client';

import {
  useTableRealtimeSession,
  type TableRealtimeMockOptions
} from '@/lib/hooks/useTableRealtimeSession';

export type {ActionError, ConnectionStatus} from '@/lib/hooks/useTableRealtimeSession';

/**
 * Public table-realtime composition boundary.
 *
 * Socket lifecycle, snapshot reconciliation and command retry bookkeeping live
 * in the session module; policy that can remain pure lives in
 * `tableResilience` and `tableNarration`. Keeping this entry point deliberately
 * small gives table consumers one stable hook while the state-machine slices
 * remain independently maintainable and testable.
 */
export function useTableRealtime(id: string, viewerId?: string, shareCode?: string,
  mockOptions?: TableRealtimeMockOptions, suppressedPlayerIds?: ReadonlySet<string>) {
  return useTableRealtimeSession(id, viewerId, shareCode, mockOptions, suppressedPlayerIds);
}
