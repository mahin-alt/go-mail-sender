import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'

import { getHealth } from '@/api/health'
import { queryKeys } from './keys'

export type ApiConnection = 'checking' | 'connected' | 'unreachable'

/** Polls /healthz so the shell can show whether the Go API is reachable. */
export function useApiHealth() {
  const query = useQuery({
    queryKey: queryKeys.health,
    queryFn: getHealth,
    refetchInterval: 30_000,
    retry: 1,
  })

  const connection = computed<ApiConnection>(() => {
    if (query.isError.value) return 'unreachable'
    if (query.isSuccess.value) return 'connected'
    return 'checking'
  })

  return { connection }
}
