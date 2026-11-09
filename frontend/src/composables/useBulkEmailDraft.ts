import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { createBatch, type NewBulkEmail } from '@/api/recipients'
import { DEFAULT_BODY, DEFAULT_SUBJECT, unknownFields } from '@/lib/template'
import { useUploadSample } from '@/queries/useUploads'
import type { UploadSummary } from '@/types/api'
import { useRecipientCheck } from './useRecipientCheck'

/** How many valid recipients the preview can flip through. */
const PREVIEW_SAMPLE = 5

/**
 * State of the New Bulk Email page: the chosen file, what the server found in it, and the message.
 * Nothing is saved or sent until `send()` is called.
 */
export function useBulkEmailDraft() {
  const check = useRecipientCheck()
  const { file, fileError, result, checking, parseError, maxUploadMb } = check
  const subject = shallowRef(DEFAULT_SUBJECT)
  const body = shallowRef(DEFAULT_BODY)
  const previewIndex = shallowRef(0)

  const create = useMutation({ mutationFn: (input: NewBulkEmail) => createBatch(input) })

  const validCount = computed(() => result.value?.valid_count ?? 0)
  const skippedCount = computed(() => result.value?.problem_count ?? 0)
  const unverifiedCount = computed(() => result.value?.unverified_count ?? 0)
  // A few real names for the preview, fetched separately: the check itself returns no rows.
  const sample = useUploadSample(() => result.value?.upload_id ?? null, PREVIEW_SAMPLE)
  const samples = computed(() => sample.data.value ?? [])
  const badFields = computed(() => unknownFields(`${subject.value}\n${body.value}`))

  const ready = computed(() => result.value !== null)
  /** Why Send is off, in words; empty when sending is possible. */
  const blocker = computed(() => {
    if (!ready.value) return 'Upload a file to begin'
    if (validCount.value === 0) return 'The file has no valid emails to send'
    if (!subject.value.trim()) return 'Write a subject'
    if (!body.value.trim()) return 'Write a message'
    if (badFields.value.length > 0) return 'Remove the unknown fields from the message'
    return ''
  })
  const canSend = computed(() => blocker.value === '')

  function chooseFile(next: File) {
    previewIndex.value = 0
    check.chooseFile(next)
  }

  function clearFile() {
    previewIndex.value = 0
    check.clearFile()
  }

  /** Start from a file that Verify Emails already checked. */
  function adopt(checkedFile: File, checked: UploadSummary) {
    previewIndex.value = 0
    check.adopt(checkedFile, checked)
  }

  function step(direction: 1 | -1) {
    const n = samples.value.length
    if (n > 0) previewIndex.value = (previewIndex.value + direction + n) % n
  }

  /** Queue the emails. Resolves with the new batch; rejects with an ApiError the page can show. */
  function send() {
    if (!file.value) return Promise.reject(new Error('No file chosen'))
    return create.mutateAsync({ file: file.value, subject: subject.value, body: body.value })
  }

  return {
    file,
    fileError,
    subject,
    body,
    previewIndex,
    result,
    checking,
    parseError,
    maxUploadMb,
    validCount,
    skippedCount,
    unverifiedCount,
    samples,
    badFields,
    ready,
    blocker,
    canSend,
    sending: computed(() => create.isPending.value),
    chooseFile,
    clearFile,
    adopt,
    step,
    send,
  }
}
