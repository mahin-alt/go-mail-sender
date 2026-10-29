import { useQuery } from '@tanstack/vue-query'

import { getHealth } from '@/api/health'
import { queryKeys } from './keys'

/** Polls GET /healthz so the sidebar can show whether the API is reachable. */
export function useApiHealth() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: getHealth,
    refetchInterval: 30_000,
    retry: false,
  })
}
