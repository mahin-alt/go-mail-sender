import { describe, expect, it } from 'vitest'

import { describeStatus } from '@/components/status/statusMeta'

describe('describeStatus', () => {
  it('gives every status a label and an icon (never colour alone)', () => {
    for (const s of [
      'pending', 'processing', 'sent', 'failed', 'cancelled', 'importing', 'import_failed', 'draft',
      'generating_jobs', 'queued', 'sending', 'completed', 'completed_with_errors',
    ] as const) {
      const meta = describeStatus(s)
      expect(meta.label).not.toBe('')
      expect(meta.icon).toBeTruthy()
    }
  })

  it('keeps failed (red) and finished-with-failures (amber) apart', () => {
    expect(describeStatus('failed').tone).toBe('bad')
    expect(describeStatus('completed_with_errors').tone).toBe('warn')
  })

  it('spins only while the server is working', () => {
    for (const s of ['processing', 'importing', 'generating_jobs', 'sending'] as const) {
      expect(describeStatus(s).spin).toBe(true)
    }
    for (const s of ['sent', 'draft', 'queued', 'completed'] as const) {
      expect(describeStatus(s).spin).toBe(false)
    }
  })

  it('tells a first-time pending job from a retry', () => {
    expect(describeStatus('pending').label).toBe('Pending')
    expect(describeStatus('pending', { attempts: 1, maxAttempts: 3 }).label).toBe('Retrying 1 of 3')
  })

  it('flags a batch that finished with failures', () => {
    expect(describeStatus('completed').label).toBe('Completed')
    expect(describeStatus('completed_with_errors').label).toBe('Completed with failures')
  })
})
