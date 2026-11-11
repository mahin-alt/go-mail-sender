import type { Component } from 'vue'
import { Ban, CircleCheck, CircleX, Clock, FilePen, LoaderCircle, TriangleAlert } from '@lucide/vue'

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
}

/**
 * One place that maps a backend status (an email's or a batch's) to wording, icon and colour, so
 * every page agrees. Colour is never the only signal: each status also has an icon and a text label.
 */
export function describeStatus(status: Status, ctx: StatusContext = {}): StatusMeta {
  const { attempts = 0, maxAttempts = 0 } = ctx
  switch (status) {
    // emails
    case 'sent':
      return { label: 'Sent', tone: 'ok', icon: CircleCheck, spin: false }
    case 'processing':
      return { label: 'Sending', tone: 'active', icon: LoaderCircle, spin: true }
    case 'pending': {
      if (attempts > 0) {
        const of = maxAttempts > 0 ? ` ${attempts} of ${maxAttempts}` : ''
        return { label: `Retrying${of}`, tone: 'warn', icon: Clock, spin: false }
      }
      return { label: 'Pending', tone: 'quiet', icon: Clock, spin: false }
    }
    // both
    case 'failed':
      return { label: 'Failed', tone: 'bad', icon: CircleX, spin: false }
    case 'cancelled':
      return { label: 'Cancelled', tone: 'quiet', icon: Ban, spin: false }
    // batches
    case 'importing':
      return { label: 'Checking file', tone: 'active', icon: LoaderCircle, spin: true }
    case 'import_failed':
      return { label: 'File not readable', tone: 'bad', icon: CircleX, spin: false }
    case 'draft':
      return { label: 'Not sent yet', tone: 'quiet', icon: FilePen, spin: false }
    case 'generating_jobs':
      return { label: 'Preparing', tone: 'active', icon: LoaderCircle, spin: true }
    case 'queued':
      return { label: 'Queued', tone: 'quiet', icon: Clock, spin: false }
    case 'sending':
      return { label: 'Sending', tone: 'active', icon: LoaderCircle, spin: true }
    case 'completed':
      return { label: 'Completed', tone: 'ok', icon: CircleCheck, spin: false }
    case 'completed_with_errors':
      return { label: 'Completed with failures', tone: 'warn', icon: TriangleAlert, spin: false }
  }
}
