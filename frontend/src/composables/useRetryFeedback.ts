import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/errors'
import { formatNumber } from '@/lib/format'

/** Toast messages for retry and cancel results. Errors stay until dismissed because they need action. */
export function useRetryFeedback() {
  const toast = useToast()

  function requeued(count: number) {
    toast.add({
      severity: 'success',
      summary: count === 1 ? 'Email requeued' : `${count} emails requeued`,
      detail: 'They will be sent again shortly.',
      life: 4000,
    })
  }

  function cancelled(count: number) {
    toast.add({
      severity: 'success',
      summary: 'Bulk Email cancelled',
      detail:
        count === 1
          ? '1 unsent email will not be sent.'
          : `${formatNumber(count)} unsent emails will not be sent.`,
      life: 4000,
    })
  }

  function failed(error: unknown, summary = 'Could not retry') {
    toast.add({
      severity: 'error',
      summary,
      detail: error instanceof ApiError ? error.message : 'Something went wrong. Please try again.',
    })
  }

  return { requeued, cancelled, failed }
}
