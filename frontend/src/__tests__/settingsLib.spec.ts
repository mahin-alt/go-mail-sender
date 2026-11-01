import { describe, expect, it } from 'vitest'

import { groupsFor, keyInMessage, SETTINGS_GROUPS } from '@/lib/settingsGroups'
import type { SettingItem } from '@/types/api'

const item = (key: string): SettingItem => ({
  key,
  description: '',
  kind: 'string',
  value: '',
  has_value: false,
  source: 'env',
  editable: true,
  secret: false,
  apply: 'instant',
  restart_required: false,
})

describe('groupsFor', () => {
  it('shows only the known groups when every key is placed', () => {
    expect(groupsFor(SETTINGS_GROUPS.flatMap((g) => g.keys).map(item)).map((g) => g.id)).toEqual([
      'smtp',
      'sending',
      'server',
      'readonly',
    ])
  })
  it('adds an Other tab for keys the page does not know', () => {
    const groups = groupsFor([item('MAIL_FROM'), item('NEW_THING')])
    expect(groups[groups.length - 1]).toMatchObject({ id: 'other', keys: ['NEW_THING'] })
  })
})

describe('keyInMessage', () => {
  const keys = ['SMTP_PORT', 'SMTP_HOST']
  it('finds the setting a refusal is about', () => {
    expect(keyInMessage('SMTP_PORT must be a whole number between 1 and 65535', keys)).toBe(
      'SMTP_PORT',
    )
  })
  it('returns null for a message about something else', () => {
    expect(keyInMessage('not saved: the new SMTP settings do not work: refused', keys)).toBeNull()
  })
})
