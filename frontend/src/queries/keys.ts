import type { BatchListParams, JobListParams } from '@/api/batches'
import type { RowKind } from '@/types/api'

/** One place for query keys so invalidation never depends on a typo. */
export const queryKeys = {
  health: ['health'] as const,
  settings: ['settings'] as const,
  stats: (days: number, tz: string) => ['stats', days, tz] as const,
  batches: {
    all: ['batches'] as const,
    list: (params: BatchListParams) => ['batches', 'list', params] as const,
    detail: (id: number) => ['batches', 'detail', id] as const,
    jobs: (id: number, params: JobListParams) => ['batches', 'jobs', id, params] as const,
  },
  uploads: {
    problems: (id: number, kinds: RowKind[], q: string) =>
      ['uploads', id, 'problems', kinds, q] as const,
    sample: (id: number) => ['uploads', id, 'sample'] as const,
  },
}
