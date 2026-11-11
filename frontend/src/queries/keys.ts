import type { BatchListParams, EmailParams } from '@/api/batches'
import type { InvalidReason } from '@/types/api'

/** One place for query keys so invalidation never depends on a typo. */
export const queryKeys = {
  health: ['health'] as const,
  settings: ['settings'] as const,
  stats: (days: number, tz: string) => ['stats', days, tz] as const,
  batches: {
    all: ['batches'] as const,
    list: (params: BatchListParams) => ['batches', 'list', params] as const,
    detail: (id: number) => ['batches', 'detail', id] as const,
    emails: (id: number, params: Omit<EmailParams, 'after'>) =>
      ['batches', 'emails', id, params] as const,
    issues: (id: number, status: string, reasons: InvalidReason[], q: string) =>
      ['batches', 'issues', id, status, reasons, q] as const,
    sample: (id: number) => ['batches', 'sample', id] as const,
  },
}
