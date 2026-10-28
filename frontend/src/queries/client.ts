import { QueryClient } from '@tanstack/vue-query'

/**
 * One shared client. Queries retry twice (network blips); mutations never retry,
 * because a repeated send or retry request must not run twice by accident.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 2, staleTime: 5_000, refetchIntervalInBackground: false },
    mutations: { retry: 0 },
  },
})
