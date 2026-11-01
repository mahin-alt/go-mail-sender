import { http } from './client'
import type { SettingItem } from '@/types/api'

export interface SettingsResponse {
  items: SettingItem[]
}

/** What to change: new values by key, and keys to put back to their environment default. */
export interface SettingsChange {
  values?: Record<string, string>
  reset?: string[]
}

/** Any field left out falls back to the server's current value, so unsaved edits can be tried. */
export interface TestEmailRequest {
  to: string
  host?: string
  port?: string
  tls_mode?: string
  username?: string
  password?: string
  from?: string
}

/** Saving SMTP settings opens a real connection first, which can take a few seconds. */
const SLOW_MS = 40_000

export async function getSettings(): Promise<SettingItem[]> {
  const { data } = await http.get<SettingsResponse>('/api/v1/settings')
  return data.items
}

export async function updateSettings(change: SettingsChange): Promise<SettingItem[]> {
  const { data } = await http.put<SettingsResponse>('/api/v1/settings', change, {
    timeout: SLOW_MS,
  })
  return data.items
}

export async function sendTestEmail(request: TestEmailRequest): Promise<string> {
  const { data } = await http.post<{ message: string }>('/api/v1/settings/smtp/test', request, {
    timeout: SLOW_MS,
  })
  return data.message
}
