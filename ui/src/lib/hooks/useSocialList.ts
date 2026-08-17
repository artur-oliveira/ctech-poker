'use client';
import {useInfiniteQuery, type QueryKey} from '@tanstack/react-query';
import type {Page} from '@/lib/api/client';

/** Cursor pagination for every social list. The server pages DynamoDB by
 * opaque cursor, so pages only ever move forward; `items` is the flattened
 * accumulation the lists render. */
export function useSocialList<T>(queryKey: QueryKey, fetchPage: (cursor?: string) => Promise<Page<T>>,
  enabled = true) {
  const query = useInfiniteQuery({
    queryKey,
    enabled,
    queryFn: ({pageParam}) => fetchPage(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page: Page<T>) => page.has_next ? page.next_cursor ?? undefined : undefined
  });
  return {
    items: query.data?.pages.flatMap(page => page.data) ?? [],
    isLoading: query.isPending && enabled,
    isError: query.isError && !query.data,
    // Cached rows with a failing refetch: worth showing, flagged as possibly behind.
    isStale: query.isError && Boolean(query.data),
    hasNext: query.hasNextPage,
    loadingMore: query.isFetchingNextPage,
    loadMore: () => void query.fetchNextPage(),
    retry: () => void query.refetch()
  };
}
