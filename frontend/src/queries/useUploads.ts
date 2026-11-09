import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useInfiniteQuery, useQuery } from '@tanstack/vue-query'

import { getUploadSample, listUploadProblems } from '@/api/uploads'
import type { RowKind } from '@/types/api'
import { queryKeys } from './keys'

/** Rows per request when paging through a checked file's problems. */
export const PROBLEM_PAGE_SIZE = 100

/**
 * A checked file's problem rows, one page at a time ("rows after N"). `fetchNextPage()` loads the
 * next page; the pages stay in memory only as far as the user has looked.
 */
export function useUploadProblems(
  uploadId: MaybeRefOrGetter<number | null>,
  kinds: MaybeRefOrGetter<RowKind[]>,
  q: MaybeRefOrGetter<string> = '',
) {
  return useInfiniteQuery({
    queryKey: computed(() =>
      queryKeys.uploads.problems(toValue(uploadId) ?? 0, toValue(kinds), toValue(q)),
    ),
    queryFn: ({ pageParam }) =>
      listUploadProblems(toValue(uploadId)!, {
        kinds: toValue(kinds),
        q: toValue(q),
        after: pageParam,
        limit: PROBLEM_PAGE_SIZE,
      }),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_after ?? undefined,
    enabled: computed(() => toValue(uploadId) != null),
    staleTime: Infinity, // a checked file never changes
  })
}

/** A few rows that will be sent, so the email preview can show real names. */
export function useUploadSample(uploadId: MaybeRefOrGetter<number | null>, limit = 5) {
  return useQuery({
    queryKey: computed(() => queryKeys.uploads.sample(toValue(uploadId) ?? 0)),
    queryFn: () => getUploadSample(toValue(uploadId)!, limit),
    enabled: computed(() => toValue(uploadId) != null),
    staleTime: Infinity,
  })
}
