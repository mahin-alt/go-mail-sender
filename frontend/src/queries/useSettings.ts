import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'

import {
  getSettings,
  sendTestEmail,
  updateSettings,
  type SettingsChange,
  type TestEmailRequest,
} from '@/api/settings'
import { queryKeys } from './keys'

/** The settings are read once and after saving; nothing else changes them, so no polling. */
export function useSettings() {
  return useQuery({ queryKey: queryKeys.settings, queryFn: getSettings })
}

/** Save and reset use the same endpoint. The reply is the refreshed list, so it replaces the cache. */
export function useUpdateSettings() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (change: SettingsChange) => updateSettings(change),
    onSuccess: (items) => client.setQueryData(queryKeys.settings, items),
  })
}

export function useSendTestEmail() {
  return useMutation({ mutationFn: (request: TestEmailRequest) => sendTestEmail(request) })
}
