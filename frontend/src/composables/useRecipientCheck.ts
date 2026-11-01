import { computed, shallowRef } from 'vue'
import { useMutation } from '@tanstack/vue-query'

import { parseRecipients } from '@/api/recipients'
import type { ParseResult } from '@/types/api'

export const MAX_FILE_BYTES = 10 * 1024 * 1024

/** Problems with the file itself, found before asking the server. */
export function fileProblem(file: Pick<File, 'name' | 'size'>): string {
  if (!/\.xlsx$/i.test(file.name)) return 'Only .xlsx files are supported.'
  if (file.size > MAX_FILE_BYTES) return 'The file is too large. The limit is 10 MB.'
  return ''
}

/**
 * The chosen spreadsheet and what the server found in it. Checking sends and saves nothing.
 * Shared by Verify Emails and New Bulk Email. The File lives in a shallowRef: a reactive proxy
 * around a File would break uploading it.
 */
export function useRecipientCheck() {
  const file = shallowRef<File | null>(null)
  const fileError = shallowRef('')
  const result = shallowRef<ParseResult | null>(null)
  const parse = useMutation({ mutationFn: (chosen: File) => parseRecipients(chosen) })

  const checking = computed(() => parse.isPending.value)
  const parseError = computed(() => parse.error.value?.message ?? '')

  function chooseFile(next: File) {
    parse.reset()
    result.value = null
    const problem = fileProblem(next)
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
  function adopt(checkedFile: File, checked: ParseResult) {
    parse.reset()
    fileError.value = ''
    file.value = checkedFile
    result.value = checked
  }

  return { file, fileError, result, checking, parseError, chooseFile, clearFile, adopt }
}
