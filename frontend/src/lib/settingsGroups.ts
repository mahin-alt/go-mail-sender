import type { SettingItem } from '@/types/api'

export interface SettingsGroup {
  id: string
  title: string
  subtitle: string
  keys: readonly string[]
}

/** The tabs of the Settings page. Which setting sits where is a UI choice; the API has no groups. */
export const SETTINGS_GROUPS: readonly SettingsGroup[] = [
  {
    id: 'smtp',
    title: 'Sender and SMTP',
    subtitle:
      'Where email is sent from. Saving connects to the SMTP server first and refuses the change if that fails.',
    keys: [
      'MAIL_FROM',
      'SMTP_HOST',
      'SMTP_PORT',
      'SMTP_TLS_MODE',
      'SMTP_USERNAME',
      'SMTP_PASSWORD',
    ],
  },
  {
    id: 'sending',
    title: 'Sending behaviour',
    subtitle: 'How recipients are checked and how many times delivery is attempted.',
    keys: ['DOMAIN_CHECK_ENABLED', 'MAX_JOB_RETRIES'],
  },
  {
    id: 'server',
    title: 'Server limits',
    subtitle: 'Saved now, but the server only reads these when it starts.',
    keys: ['WORKER_COUNT', 'SMTP_POOL_SIZE', 'JOB_POLL_INTERVAL'],
  },
  {
    id: 'readonly',
    title: 'Read-only',
    subtitle: 'Shown for reference. These can only be changed in the server environment.',
    keys: ['HTTP_ADDR', 'HTTP_READ_TIMEOUT', 'HTTP_WRITE_TIMEOUT'],
  },
]

const OTHER: SettingsGroup = {
  id: 'other',
  title: 'Other',
  subtitle: 'Settings the server offers that this page does not know about yet.',
  keys: [],
}

/** The groups to show: the known ones, plus an "Other" tab if the server sent keys we do not place. */
export function groupsFor(items: readonly SettingItem[]): SettingsGroup[] {
  const known = new Set(SETTINGS_GROUPS.flatMap((g) => g.keys))
  const extra = items.filter((i) => !known.has(i.key)).map((i) => i.key)
  return extra.length ? [...SETTINGS_GROUPS, { ...OTHER, keys: extra }] : [...SETTINGS_GROUPS]
}

/**
 * A rejected save names the setting it is about at the start of the message
 * ("SMTP_PORT must be a whole number…"), so the message can sit under that field.
 */
export function keyInMessage(message: string, keys: readonly string[]): string | null {
  return keys.find((key) => message.startsWith(key) || message.startsWith(`${key}:`)) ?? null
}
