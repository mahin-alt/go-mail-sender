import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from '@tanstack/vue-query'

import {
  cancelBatch,
  getBatch,
  getSample,
  listBatches,
  listEmails,
  listIssues,
  retryBatch,
  retryJob,
  sendBatch,
  type BatchListParams,
  type EmailParams,
} from '@/api/batches'
import type { Batch, BatchStatus, InvalidReason } from '@/types/api'
import { queryKeys } from './keys'
import { POLL_MS } from './polling'

/** Statuses in which the server is busy with a batch right now. */
export const WORKING: readonly BatchStatus[] = ['importing', 'generating_jobs', 'queued', 'sending']

/** The page of batches for the current filters. Keeps the old rows on screen while a new page loads. */
export function useBatches(params: MaybeRefOrGetter<BatchListParams>) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.list(toValue(params))),
    queryFn: () => listBatches(toValue(params)),
    refetchInterval: POLL_MS.list,
    placeholderData: keepPreviousData,
  })
}

/** The Bulk Emails list tabs, each with the statuses it stands for. */
export const LIST_TABS = ['', 'draft', 'sending', 'completed', 'completed_with_errors', 'cancelled'] as const
export type ListTab = (typeof LIST_TABS)[number]

/**
 * Counts for the Bulk Emails status tabs: one tiny request (limit 1) per tab, read from `total`.
 * Returns a map keyed by tab, `''` being "All"; a count is `undefined` until it loads.
 */
export function useBatchStatusCounts() {
  const results = useQueries({
    queries: LIST_TABS.map((status) => {
      const params: BatchListParams = { type: 'bulk_send', status, q: '', limit: 1, offset: 0 }
      return {
        queryKey: queryKeys.batches.list(params),
        queryFn: () => listBatches(params),
        refetchInterval: POLL_MS.list,
      }
    }),
  })
  return computed(() => {
    const counts: Partial<Record<ListTab, number>> = {}
    LIST_TABS.forEach((status, i) => {
      counts[status] = results.value[i]?.data?.total
    })
    return counts
  })
}

/**
 * One batch, polled: every second while its file is being imported (the progress counter moves
 * fast), every few seconds otherwise. Disabled while `id` is null.
 */
export function useBatch(id: MaybeRefOrGetter<number | null>) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.detail(toValue(id) ?? 0)),
    queryFn: () => getBatch(toValue(id)!),
    enabled: computed(() => toValue(id) != null),
    refetchInterval: (query) =>
      (query.state.data as Batch | undefined)?.status === 'importing' ? POLL_MS.importing : POLL_MS.detail,
  })
}

/** Rows per request when paging through problem rows. */
export const ISSUE_PAGE_SIZE = 100

/**
 * A batch's problem rows, one page at a time. `fetchNextPage()` loads the next page; pages stay in
 * memory only as far as the user has looked. Only call once the import is done (rows are final).
 */
export function useIssues(
  batchId: MaybeRefOrGetter<number | null>,
  filter: MaybeRefOrGetter<{ status: 'invalid' | 'unverified'; reasons: InvalidReason[] }>,
  q: MaybeRefOrGetter<string> = '',
) {
  return useInfiniteQuery({
    queryKey: computed(() => {
      const f = toValue(filter)
      return queryKeys.batches.issues(toValue(batchId) ?? 0, f.status, f.reasons, toValue(q))
    }),
    queryFn: ({ pageParam }) => {
      const f = toValue(filter)
      return listIssues(toValue(batchId)!, {
        status: f.status,
        reasons: f.reasons,
        q: toValue(q),
        after: pageParam,
        limit: ISSUE_PAGE_SIZE,
      })
    },
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_after ?? undefined,
    enabled: computed(() => toValue(batchId) != null),
    staleTime: Infinity, // an imported file's rows never change
  })
}

/** A few rows that will be sent, so the email preview can show real names. */
export function useSample(batchId: MaybeRefOrGetter<number | null>, limit = 5) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.sample(toValue(batchId) ?? 0)),
    queryFn: () => getSample(toValue(batchId)!, limit),
    enabled: computed(() => toValue(batchId) != null),
    staleTime: Infinity,
  })
}

/** Emails per page in the delivery list. */
export const EMAIL_PAGE_SIZE = 20

/** A batch's emails for the current filters, a page at a time ("Load more"), polled. */
export function useEmails(
  id: MaybeRefOrGetter<number>,
  params: MaybeRefOrGetter<Omit<EmailParams, 'after' | 'limit'>>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  return useInfiniteQuery({
    queryKey: computed(() =>
      queryKeys.batches.emails(toValue(id), { ...toValue(params), limit: EMAIL_PAGE_SIZE }),
    ),
    queryFn: ({ pageParam }) =>
      listEmails(toValue(id), { ...toValue(params), after: pageParam, limit: EMAIL_PAGE_SIZE }),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_after ?? undefined,
    enabled: computed(() => toValue(enabled)),
    refetchInterval: POLL_MS.detail,
    placeholderData: keepPreviousData,
  })
}

/** Send a checked batch. Refreshes all batch data afterwards. */
export function useSendBatch() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: number; subject: string; body: string }) =>
      sendBatch(input.id, { subject: input.subject, body: input.body }),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}

/** Requeue every failed email of one batch. Refreshes all batch data afterwards. */
export function useRetryBatch() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => retryBatch(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}

/** Stop a batch that is still sending. Refreshes all batch data afterwards. */
export function useCancelBatch() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => cancelBatch(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}

/** Requeue one failed email. */
export function useRetryJob() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => retryJob(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}
