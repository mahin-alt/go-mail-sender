import { describe, expect, it } from 'vitest'

import { describeStatus } from '@/components/status/statusMeta'

describe('describeStatus', () => {
  it('gives every status a label and an icon (never colour alone)', () => {
    for (const s of ['pending', 'processing', 'sent', 'failed', 'invalid', 'completed'] as const) {
      const meta = describeStatus(s)
      expect(meta.label).not.toBe('')
      expect(meta.icon).toBeTruthy()
    }
  })

  it('keeps failed (red) and invalid (amber) apart', () => {
    expect(describeStatus('failed').tone).toBe('bad')
    expect(describeStatus('invalid').tone).toBe('warn')
  })

  it('spins only while sending', () => {
    expect(describeStatus('processing').spin).toBe(true)
    expect(describeStatus('sent').spin).toBe(false)
  })

  it('tells a first-time pending job from a retry', () => {
    expect(describeStatus('pending').label).toBe('Pending')
    expect(describeStatus('pending', { attempts: 1, maxAttempts: 3 }).label).toBe('Retrying 1 of 3')
  })

  it('flags a completed batch that has failures', () => {
    expect(describeStatus('completed').label).toBe('Completed')
    expect(describeStatus('completed', { failed: 12 }).label).toBe('Completed with failures')
  })
})
