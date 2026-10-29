import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  keepPreviousData,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from '@tanstack/vue-query'

import {
  getBatch,
  listBatches,
  listJobs,
  retryBatch,
  retryJob,
  type BatchListParams,
  type JobListParams,
} from '@/api/batches'
import type { BatchStatus } from '@/types/api'
import { queryKeys } from './keys'
import { POLL_MS } from './polling'

/** The page of batches for the current filters. Keeps the old rows on screen while a new page loads. */
export function useBatches(params: MaybeRefOrGetter<BatchListParams>) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.list(toValue(params))),
    queryFn: () => listBatches(toValue(params)),
    refetchInterval: POLL_MS.list,
    placeholderData: keepPreviousData,
  })
}

const COUNTED: (BatchStatus | '')[] = ['', 'pending', 'processing', 'completed']

/**
 * Counts for the status tabs: one tiny request (limit 1) per tab, read from `total`.
 * Returns a map keyed by status, `''` being "All"; a count is `undefined` until it loads.
 */
export function useBatchStatusCounts() {
  const results = useQueries({
    queries: COUNTED.map((status) => {
      const params: BatchListParams = { status, q: '', limit: 1, offset: 0 }
      return {
        queryKey: queryKeys.batches.list(params),
        queryFn: () => listBatches(params),
        refetchInterval: POLL_MS.list,
      }
    }),
  })
  return computed(() => {
    const counts: Partial<Record<BatchStatus | '', number>> = {}
    COUNTED.forEach((status, i) => {
      counts[status] = results.value[i]?.data?.total
    })
    return counts
  })
}

export function useBatch(id: MaybeRefOrGetter<number>) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.detail(toValue(id))),
    queryFn: () => getBatch(toValue(id)),
    refetchInterval: POLL_MS.detail,
  })
}

export function useJobs(id: MaybeRefOrGetter<number>, params: MaybeRefOrGetter<JobListParams>) {
  return useQuery({
    queryKey: computed(() => queryKeys.batches.jobs(toValue(id), toValue(params))),
    queryFn: () => listJobs(toValue(id), toValue(params)),
    refetchInterval: POLL_MS.detail,
    placeholderData: keepPreviousData,
  })
}

/** Requeue every failed job of one batch. Refreshes all batch data afterwards. */
export function useRetryBatch() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => retryBatch(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}

/** Requeue one failed job. */
export function useRetryJob() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => retryJob(id),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.batches.all }),
  })
}
