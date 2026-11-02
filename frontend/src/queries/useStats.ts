import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { keepPreviousData, useQuery } from '@tanstack/vue-query'

import { getStats } from '@/api/stats'
import { timeZone } from '@/lib/dashboard'
import { queryKeys } from './keys'
import { POLL_MS } from './polling'

/** The Dashboard numbers for the last `days` days. Keeps the old numbers on screen while a new range loads. */
export function useStats(days: MaybeRefOrGetter<number>) {
  const tz = timeZone()
  return useQuery({
    queryKey: computed(() => queryKeys.stats(toValue(days), tz)),
    queryFn: () => getStats({ days: toValue(days), tz }),
    refetchInterval: POLL_MS.list,
    placeholderData: keepPreviousData,
  })
}
