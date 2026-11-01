import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { createBatch, parseRecipients, type NewBulkEmail } from '@/api/recipients'
import { DEFAULT_BODY, DEFAULT_SUBJECT, unknownFields } from '@/lib/template'

export const MAX_FILE_BYTES = 10 * 1024 * 1024

/** How many valid recipients the preview can flip through. */
const PREVIEW_SAMPLE = 5

/** Problems with the file itself, found before asking the server. */
export function fileProblem(file: Pick<File, 'name' | 'size'>): string {
  if (!/\.xlsx$/i.test(file.name)) return 'Only .xlsx files are supported.'
  if (file.size > MAX_FILE_BYTES) return 'The file is too large. The limit is 10 MB.'
  return ''
}

/**
 * State of the New Bulk Email page: the chosen file, what the server found in it, and the message.
 * Nothing is saved or sent until `send()` is called.
 */
export function useBulkEmailDraft() {
  const file = shallowRef<File | null>(null)
  const fileError = shallowRef('')
  const subject = shallowRef(DEFAULT_SUBJECT)
  const body = shallowRef(DEFAULT_BODY)
  const previewIndex = shallowRef(0)

  const parse = useMutation({ mutationFn: (chosen: File) => parseRecipients(chosen) })
  const create = useMutation({ mutationFn: (input: NewBulkEmail) => createBatch(input) })

  const result = computed(() => parse.data.value ?? null)
  const checking = computed(() => parse.isPending.value)
  const parseError = computed(() => parse.error.value?.message ?? '')
  const validCount = computed(() => result.value?.valid.length ?? 0)
  const skippedCount = computed(() => result.value?.problems.length ?? 0)
  const unverifiedCount = computed(() => result.value?.unverified_count ?? 0)
  const samples = computed(() => result.value?.valid.slice(0, PREVIEW_SAMPLE) ?? [])
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
    const problem = fileProblem(next)
    parse.reset()
    previewIndex.value = 0
    if (problem) {
      fileError.value = problem
      file.value = null
      return
    }
    fileError.value = ''
    file.value = next
    parse.mutate(next)
  }

  function clearFile() {
    file.value = null
    fileError.value = ''
    previewIndex.value = 0
    parse.reset()
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
    step,
    send,
  }
}
