import { describe, expect, it } from 'vitest'

import { DEFAULT_MAX_UPLOAD_MB, uploadLimitMb } from '@/lib/uploadLimit'
import type { SettingItem } from '@/types/api'

const setting = (key: string, value: string): SettingItem => ({
  key,
  description: '',
  kind: 'int',
  value,
  has_value: value !== '',
  source: 'env',
  editable: true,
  secret: false,
  apply: 'instant',
  restart_required: false,
})

describe('uploadLimitMb', () => {
  it('reads MAX_UPLOAD_MB', () => {
    expect(uploadLimitMb([setting('MAIL_FROM', 'a@b.c'), setting('MAX_UPLOAD_MB', '40')])).toBe(40)
  })
  it('falls back to the default before settings load or when the value is unusable', () => {
    expect(uploadLimitMb(undefined)).toBe(DEFAULT_MAX_UPLOAD_MB)
    expect(uploadLimitMb([])).toBe(DEFAULT_MAX_UPLOAD_MB)
    expect(uploadLimitMb([setting('MAX_UPLOAD_MB', '')])).toBe(DEFAULT_MAX_UPLOAD_MB)
    expect(uploadLimitMb([setting('MAX_UPLOAD_MB', '0')])).toBe(DEFAULT_MAX_UPLOAD_MB)
    expect(uploadLimitMb([setting('MAX_UPLOAD_MB', '2.5')])).toBe(DEFAULT_MAX_UPLOAD_MB)
  })
})
