import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { createUpload } from '@/api/uploads'
import { uploadLimitMb } from '@/lib/uploadLimit'
import { useSettings } from '@/queries/useSettings'
import type { UploadSummary } from '@/types/api'

/** Problems with the file itself, found before asking the server. */
export function fileProblem(file: Pick<File, 'name' | 'size'>, maxMb: number): string {
  if (!/\.xlsx$/i.test(file.name)) return 'Only .xlsx files are supported.'
  if (file.size > maxMb * 1024 * 1024) return `The file is too large. The limit is ${maxMb} MB.`
  return ''
}

/**
 * The chosen spreadsheet and what the server found in it (totals only; problem rows are paged from
 * the server). Checking sends nothing; the server keeps the result for 24 hours.
 * Shared by Verify Emails and New Bulk Email. The File lives in a shallowRef: a reactive proxy
 * around a File would break uploading it.
 */
export function useRecipientCheck() {
  const file = shallowRef<File | null>(null)
  const fileError = shallowRef('')
  const result = shallowRef<UploadSummary | null>(null)
  const parse = useMutation({ mutationFn: (chosen: File) => createUpload(chosen) })
  // MAX_UPLOAD_MB is changed on the Settings page without a restart; re-read whenever a page using this opens.
  const settings = useSettings()
  const maxUploadMb = computed(() => uploadLimitMb(settings.data.value))

  const checking = computed(() => parse.isPending.value)
  const parseError = computed(() => parse.error.value?.message ?? '')

  function chooseFile(next: File) {
    parse.reset()
    result.value = null
    const problem = fileProblem(next, maxUploadMb.value)
    if (problem) {
      fileError.value = problem
      file.value = null
      return
    }
    fileError.value = ''
    file.value = next
    parse.mutate(next, {
      onSuccess: (checked) => {
        if (file.value === next) result.value = checked // ignore an answer for a file that was replaced
      },
    })
  }

  function clearFile() {
    parse.reset()
    file.value = null
    fileError.value = ''
    result.value = null
  }

  /** Take over a file that was already checked elsewhere, so it is not uploaded and checked twice. */
  function adopt(checkedFile: File, checked: UploadSummary) {
    parse.reset()
    fileError.value = ''
    file.value = checkedFile
    result.value = checked
  }

  return {
    file,
    fileError,
    result,
    checking,
    parseError,
    maxUploadMb,
    chooseFile,
    clearFile,
    adopt,
  }
}
