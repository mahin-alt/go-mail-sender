import { defineStore } from 'pinia'
import { shallowRef } from 'vue'

import type { ParseResult } from '@/types/api'

/**
 * A checked spreadsheet handed from Verify Emails to New Bulk Email, so the user does not have to
 * choose and check the file twice. It is taken once. shallowRef keeps the File a real File.
 */
export const usePendingCheckStore = defineStore('pendingCheck', () => {
  const file = shallowRef<File | null>(null)
  const result = shallowRef<ParseResult | null>(null)

  function hold(checkedFile: File, checked: ParseResult) {
    file.value = checkedFile
    result.value = checked
  }

  /** Returns the held file and its result, and forgets them. */
  function take(): { file: File; result: ParseResult } | null {
    const taken = file.value && result.value ? { file: file.value, result: result.value } : null
    file.value = null
    result.value = null
    return taken
  }

  return { hold, take }
})
