import { QueryClient } from '@tanstack/vue-query'

import { ApiError } from '@/api/errors'

/** Retry twice for network blips and 5xx, but never for a 4xx (a 404 will not fix itself). */
function retryUnlessClientError(count: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false
  return count < 2
}

/**
 * One shared client. Queries retry twice (network blips); mutations never retry,
 * because a repeated send or retry request must not run twice by accident.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: retryUnlessClientError,
      staleTime: 5_000,
      refetchIntervalInBackground: false,
    },
    mutations: { retry: 0 },
  },
})
