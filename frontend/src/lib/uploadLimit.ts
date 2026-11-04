import type { SettingItem } from '@/types/api'

/** The server's default, used until the settings have loaded. The server always has the final say. */
export const DEFAULT_MAX_UPLOAD_MB = 10

/** The upload size limit from the MAX_UPLOAD_MB setting, in MB. */
export function uploadLimitMb(items: readonly SettingItem[] | undefined): number {
  const value = Number(items?.find((i) => i.key === 'MAX_UPLOAD_MB')?.value)
  return Number.isInteger(value) && value > 0 ? value : DEFAULT_MAX_UPLOAD_MB
}
