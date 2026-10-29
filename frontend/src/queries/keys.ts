import type { BatchListParams, JobListParams } from '@/api/batches'

/** One place for query keys so invalidation never depends on a typo. */
export const queryKeys = {
  health: ['health'] as const,
  batches: {
    all: ['batches'] as const,
    list: (params: BatchListParams) => ['batches', 'list', params] as const,
    detail: (id: number) => ['batches', 'detail', id] as const,
    jobs: (id: number, params: JobListParams) => ['batches', 'jobs', id, params] as const,
  },
}
