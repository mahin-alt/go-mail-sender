import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { uploadSpreadsheet } from '@/api/batches'
import { formatNumber } from '@/lib/format'
import { uploadLimitMb } from '@/lib/uploadLimit'
import { useBatch } from '@/queries/useBatches'
import { useSettings } from '@/queries/useSettings'
import type { BatchType } from '@/types/api'

/** Problems with the file itself, found before asking the server. */
export function fileProblem(file: Pick<File, 'name' | 'size'>, maxMb: number): string {
  if (!/\.xlsx$/i.test(file.name)) return 'Only .xlsx files are supported.'
  if (file.size > maxMb * 1024 * 1024) return `The file is too large. The limit is ${maxMb} MB.`
  return ''
}

/** What the page shows about the file: its name, and its size when it was chosen here. */
export interface ChosenFile {
  name: string
  size?: number
}

/**
 * A spreadsheet on its way to becoming a checked batch, shared by Verify Emails and New Bulk
 * Email. Three steps the user can tell apart:
 *   1. uploading  - the browser sends the file (with a percentage)
 *   2. importing  - the server reads and checks every row in the background (rows counted)
 *   3. ready      - the batch's totals are final; problem rows are paged from the server
 * Nothing is sent. The server keeps the batch, so leaving the page loses nothing: the batch can be
 * opened again by its ID (`adopt`).
 */
export function useSpreadsheetUpload(type: BatchType) {
  const file = shallowRef<ChosenFile | null>(null)
  const fileError = shallowRef('')
  const batchId = shallowRef<number | null>(null)
  const uploaded = shallowRef(0) // fraction of the file sent so far, 0..1
  let choice = 0 // which file choice an upload answer belongs to

  const upload = useMutation({
    mutationFn: (chosen: File) =>
      uploadSpreadsheet(type, chosen, (fraction) => (uploaded.value = fraction)),
  })
  const batchQuery = useBatch(batchId)
  const batch = computed(() => (batchId.value != null ? batchQuery.data.value : undefined))

  // MAX_UPLOAD_MB is changed on the Settings page without a restart; re-read whenever a page using this opens.
  const settings = useSettings()
  const maxUploadMb = computed(() => uploadLimitMb(settings.data.value))

  const uploading = computed(() => upload.isPending.value)
  const importing = computed(
    () => batchId.value != null && (!batch.value || batch.value.status === 'importing'),
  )
  /** The checked batch, once its import finished successfully. */
  const result = computed(() =>
    batch.value && batch.value.status !== 'importing' && batch.value.status !== 'import_failed'
      ? batch.value
      : null,
  )
  const busy = computed(() => uploading.value || importing.value)
  /** "Uploading 42%" or "Checking rows: 120,000 read". */
  const busyText = computed(() => {
    if (uploading.value) return `Uploading ${Math.round(uploaded.value * 100)}%`
    const read = batch.value?.processed_rows ?? 0
    return read > 0
      ? `Checking every row: ${formatNumber(read)} read so far. Nothing is sent yet.`
      : 'Checking every row. Nothing is sent yet.'
  })
  /** A problem the user must act on: a bad file, a failed upload, a failed import. */
  const error = computed(() => {
    if (fileError.value) return fileError.value
    if (upload.error.value) return upload.error.value.message
    if (batch.value?.status === 'import_failed') {
      return `The file could not be checked: ${batch.value.import_error ?? 'unknown error'}`
    }
    if (batchQuery.error.value && batchId.value != null) return batchQuery.error.value.message
    return ''
  })

  function chooseFile(next: File) {
    const mine = ++choice
    upload.reset()
    batchId.value = null
    uploaded.value = 0
    const problem = fileProblem(next, maxUploadMb.value)
    if (problem) {
      fileError.value = problem
      file.value = null
      return
    }
    fileError.value = ''
    file.value = { name: next.name, size: next.size }
    upload.mutate(next, {
      onSuccess: (res) => {
        if (mine === choice) batchId.value = res.batch_id // ignore the answer for a replaced file
      },
    })
  }

  function clearFile() {
    choice++
    upload.reset()
    file.value = null
    fileError.value = ''
    batchId.value = null
    uploaded.value = 0
  }

  /** Continue with a batch that was uploaded before (from Verify Emails or the Bulk Emails list). */
  function adopt(id: number, filename = '') {
    choice++
    upload.reset()
    fileError.value = ''
    batchId.value = id
    file.value = { name: filename || `Batch #${id}` }
  }

  /** The file to show: an adopted batch shows its filename once it has loaded. */
  const shownFile = computed<ChosenFile | null>(() =>
    file.value && file.value.size === undefined && batch.value?.filename
      ? { name: batch.value.filename }
      : file.value,
  )

  return {
    file: shownFile,
    batchId,
    batch,
    result,
    uploading,
    importing,
    busy,
    busyText,
    error,
    maxUploadMb,
    chooseFile,
    clearFile,
    adopt,
  }
}
