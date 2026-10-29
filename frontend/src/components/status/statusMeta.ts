import type { Component } from 'vue'
import { CircleCheck, CircleX, Clock, LoaderCircle, TriangleAlert } from '@lucide/vue'

import type { BatchStatus, JobStatus } from '@/types/api'

export type Status = JobStatus | BatchStatus
export type Tone = 'ok' | 'bad' | 'warn' | 'active' | 'quiet'

export interface StatusMeta {
  label: string
  tone: Tone
  icon: Component
  /** True when the icon should rotate (only while something is actually in progress). */
  spin: boolean
}

export interface StatusContext {
  /** Job attempts so far and the maximum, to tell "pending" from "retrying". */
  attempts?: number
  maxAttempts?: number
  /** Batch failed-job count, to show "Completed with failures". */
  failed?: number
}

/**
 * One place that maps a backend status to wording, icon and colour, so every page agrees.
 * Colour is never the only signal: each status also has an icon and a text label.
 * `invalid` is amber and `failed` is red on purpose (different actions: fix the file vs retry).
 */
export function describeStatus(status: Status, ctx: StatusContext = {}): StatusMeta {
  const { attempts = 0, maxAttempts = 0, failed = 0 } = ctx
  switch (status) {
    case 'sent':
      return { label: 'Sent', tone: 'ok', icon: CircleCheck, spin: false }
    case 'completed':
      return failed > 0
        ? { label: 'Completed with failures', tone: 'warn', icon: TriangleAlert, spin: false }
        : { label: 'Completed', tone: 'ok', icon: CircleCheck, spin: false }
    case 'failed':
      return { label: 'Failed', tone: 'bad', icon: CircleX, spin: false }
    case 'invalid':
      return { label: 'Skipped (invalid)', tone: 'warn', icon: TriangleAlert, spin: false }
    case 'processing':
      return { label: 'Sending', tone: 'active', icon: LoaderCircle, spin: true }
    case 'pending': {
      if (attempts > 0) {
        const of = maxAttempts > 0 ? ` ${attempts} of ${maxAttempts}` : ''
        return { label: `Retrying${of}`, tone: 'warn', icon: Clock, spin: false }
      }
      return { label: 'Pending', tone: 'quiet', icon: Clock, spin: false }
    }
  }
}
