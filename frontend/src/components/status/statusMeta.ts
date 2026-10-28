import type { Component } from 'vue'
import { CircleCheck, CircleX, Clock, LoaderCircle, TriangleAlert } from '@lucide/vue'

import type { BatchStatus, JobStatus } from '@/types/api'

export type Status = JobStatus | BatchStatus
export type Severity = 'success' | 'danger' | 'warn' | 'info' | 'secondary'

export interface StatusMeta {
  label: string
  severity: Severity
  icon: Component
  /** True when the icon should rotate (only while something is actually in progress). */
  spin: boolean
}

/**
 * One place that maps a status to its colour, icon and wording, so every table agrees.
 * Colour is never the only signal: each status also has an icon and a text label.
 * `invalid` is amber and `failed` is red on purpose (different actions: fix the file vs retry).
 */
export function describeStatus(status: Status, attempts = 0, maxAttempts = 0): StatusMeta {
  switch (status) {
    case 'sent':
      return { label: 'Sent', severity: 'success', icon: CircleCheck, spin: false }
    case 'completed':
      return { label: 'Completed', severity: 'success', icon: CircleCheck, spin: false }
    case 'failed':
      return { label: 'Failed', severity: 'danger', icon: CircleX, spin: false }
    case 'invalid':
      return { label: 'Skipped (invalid)', severity: 'warn', icon: TriangleAlert, spin: false }
    case 'processing':
      return { label: 'Sending', severity: 'info', icon: LoaderCircle, spin: true }
    case 'pending': {
      // A pending job that has already been tried is waiting for another attempt.
      if (attempts > 0) {
        const of = maxAttempts > 0 ? ` ${attempts} of ${maxAttempts}` : ''
        return { label: `Retrying${of}`, severity: 'warn', icon: Clock, spin: false }
      }
      return { label: 'Pending', severity: 'secondary', icon: Clock, spin: false }
    }
  }
}
