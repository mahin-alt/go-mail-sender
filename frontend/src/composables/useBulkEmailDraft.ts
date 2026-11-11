import { computed, shallowRef } from 'vue'

import { DEFAULT_BODY, DEFAULT_SUBJECT, unknownFields } from '@/lib/template'
import { useSample, useSendBatch } from '@/queries/useBatches'
import { useSpreadsheetUpload } from './useSpreadsheetUpload'

/** How many valid recipients the preview can flip through. */
const PREVIEW_SAMPLE = 5

/**
 * State of the New Bulk Email page: the uploaded file (a draft batch on the server), what the
 * server found in it, and the message. Nothing is sent until `send()` is called.
 */
export function useBulkEmailDraft() {
  const upload = useSpreadsheetUpload('bulk_send')
  const { result } = upload
  const subject = shallowRef(DEFAULT_SUBJECT)
  const body = shallowRef(DEFAULT_BODY)
  const previewIndex = shallowRef(0)

  const sendMutation = useSendBatch()

  const validCount = computed(() => (result.value ? result.value.valid_count + result.value.unverified_count : 0))
  const skippedCount = computed(() => result.value?.invalid_count ?? 0)
  const unverifiedCount = computed(() => result.value?.unverified_count ?? 0)
  // A few real names for the preview, fetched separately: the batch itself carries no rows.
  const sample = useSample(() => result.value?.id ?? null, PREVIEW_SAMPLE)
  const samples = computed(() => sample.data.value ?? [])
  const badFields = computed(() => unknownFields(`${subject.value}\n${body.value}`))

  const ready = computed(() => result.value !== null)
  /** A draft, or a checked file from Verify Emails: anything else was sent already. */
  const sendable = computed(() => {
    const b = result.value
    return !!b && (b.status === 'draft' || (b.type === 'verification' && b.status === 'completed'))
  })
  /** Why Send is off, in words; empty when sending is possible. */
  const blocker = computed(() => {
    if (!ready.value) return 'Upload a file to begin'
    if (!sendable.value) return 'This file was already sent'
    if (validCount.value === 0) return 'The file has no valid emails to send'
    if (!subject.value.trim()) return 'Write a subject'
    if (!body.value.trim()) return 'Write a message'
    if (badFields.value.length > 0) return 'Remove the unknown fields from the message'
    return ''
  })
  const canSend = computed(() => blocker.value === '')

  function chooseFile(next: File) {
    previewIndex.value = 0
    upload.chooseFile(next)
  }

  function clearFile() {
    previewIndex.value = 0
    upload.clearFile()
  }

  /** Start from a batch uploaded before (Verify Emails, or a draft from the list). */
  function adopt(id: number, filename = '') {
    previewIndex.value = 0
    upload.adopt(id, filename)
  }

  function step(direction: 1 | -1) {
    const n = samples.value.length
    if (n > 0) previewIndex.value = (previewIndex.value + direction + n) % n
  }

  /** Start sending. Resolves with the batch; rejects with an ApiError the page can show. */
  function send() {
    const id = result.value?.id
    if (id == null) return Promise.reject(new Error('No file uploaded'))
    return sendMutation.mutateAsync({ id, subject: subject.value, body: body.value })
  }

  return {
    ...upload,
    subject,
    body,
    previewIndex,
    validCount,
    skippedCount,
    unverifiedCount,
    samples,
    badFields,
    ready,
    blocker,
    canSend,
    sending: computed(() => sendMutation.isPending.value),
    chooseFile,
    clearFile,
    adopt,
    step,
    send,
  }
}
