import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/errors'

/** Toast messages for retry results. Errors stay until dismissed because they need action. */
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

  function failed(error: unknown) {
    toast.add({
      severity: 'error',
      summary: 'Could not retry',
      detail: error instanceof ApiError ? error.message : 'Something went wrong. Please try again.',
    })
  }

  return { requeued, failed }
}
