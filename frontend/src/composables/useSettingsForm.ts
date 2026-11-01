import { computed, reactive, type Ref } from 'vue'

import type { SettingsChange } from '@/api/settings'
import type { SettingItem } from '@/types/api'

/** A secret is never shown. The user either leaves it, types a new one, or removes it. */
interface SecretDraft {
  mode: 'keep' | 'change' | 'remove'
  value: string
}

/**
 * Edits made on the Settings page but not yet saved. The server's values stay the source of truth;
 * a draft exists only for a setting that was changed, so "dirty" is simply "has a draft".
 */
export function useSettingsForm(items: Ref<SettingItem[]>) {
  const drafts = reactive<Record<string, string>>({})
  const secrets = reactive<Record<string, SecretDraft>>({})

  const byKey = computed(() => new Map(items.value.map((item) => [item.key, item])))

  function valueOf(key: string): string {
    return drafts[key] ?? byKey.value.get(key)?.value ?? ''
  }

  function setValue(key: string, value: string) {
    if (value === byKey.value.get(key)?.value) delete drafts[key]
    else drafts[key] = value
  }

  function secretOf(key: string): SecretDraft {
    return secrets[key] ?? { mode: 'keep', value: '' }
  }

  function changePassword(key: string) {
    secrets[key] = { mode: 'change', value: '' }
  }
  function typePassword(key: string, value: string) {
    secrets[key] = { mode: 'change', value }
  }
  function removePassword(key: string) {
    secrets[key] = { mode: 'remove', value: '' }
  }
  function keepPassword(key: string) {
    delete secrets[key]
  }

  function isDirty(key: string): boolean {
    if (key in drafts) return true
    const secret = secrets[key]
    return (
      !!secret && (secret.mode === 'remove' || (secret.mode === 'change' && secret.value !== ''))
    )
  }

  function dirtyKeys(keys: readonly string[]): string[] {
    return keys.filter(isDirty)
  }

  /** The request body for saving these settings: only what changed. An empty string removes a secret. */
  function changeFor(keys: readonly string[]): SettingsChange {
    const values: Record<string, string> = {}
    for (const key of dirtyKeys(keys)) {
      const secret = secrets[key]
      values[key] = secret
        ? secret.mode === 'remove'
          ? ''
          : secret.value
        : (drafts[key] as string)
    }
    return { values }
  }

  function discard(keys: readonly string[]) {
    for (const key of keys) {
      delete drafts[key]
      delete secrets[key]
    }
  }

  return {
    valueOf,
    setValue,
    secretOf,
    changePassword,
    typePassword,
    removePassword,
    keepPassword,
    isDirty,
    dirtyKeys,
    changeFor,
    discard,
  }
}
