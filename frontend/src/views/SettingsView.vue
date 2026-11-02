<script setup lang="ts">
import { computed, reactive, shallowRef } from 'vue'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/errors'
import PageHeader from '@/components/layout/PageHeader.vue'
import SettingRow from '@/components/settings/SettingRow.vue'
import SettingsTabBar from '@/components/settings/SettingsTabBar.vue'
import TestEmailBox from '@/components/settings/TestEmailBox.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { useSettingsForm } from '@/composables/useSettingsForm'
import { groupsFor, keyInMessage, type SettingsGroup } from '@/lib/settingsGroups'
import { useSettings, useUpdateSettings } from '@/queries/useSettings'
import type { SettingItem } from '@/types/api'

const settings = useSettings()
const items = computed(() => settings.data.value ?? [])
const byKey = computed(() => new Map(items.value.map((item) => [item.key, item])))
const groups = computed(() => groupsFor(items.value))

const form = useSettingsForm(items)
const update = useUpdateSettings()
const toast = useToast()

const activeId = shallowRef('smtp')
const active = computed(() => groups.value.find((g) => g.id === activeId.value) ?? groups.value[0])

/** Why the last save was refused: shown under the field it names, else above the buttons. */
const groupError = reactive<Record<string, string>>({})
const fieldError = reactive<Record<string, string>>({})
const savingGroup = shallowRef('')

const tabs = computed(() =>
  groups.value.map((g) => ({ id: g.id, label: g.title, dirty: form.dirtyKeys(g.keys).length > 0 })),
)

function rowsOf(group: SettingsGroup): SettingItem[] {
  return group.keys.flatMap((key) => byKey.value.get(key) ?? [])
}
const editable = (group: SettingsGroup) => rowsOf(group).some((item) => item.editable)
const restartPending = computed(() => items.value.some((item) => item.restart_required))

function clearErrors(group: SettingsGroup) {
  delete groupError[group.id]
  for (const key of group.keys) delete fieldError[key]
}

function showError(group: SettingsGroup, error: unknown) {
  const message =
    error instanceof ApiError ? error.message : 'Something went wrong. Nothing was saved.'
  const key = keyInMessage(message, group.keys)
  if (key) fieldError[key] = message
  else groupError[group.id] = message
}

function save(group: SettingsGroup) {
  const change = form.changeFor(group.keys)
  clearErrors(group)
  savingGroup.value = group.id
  update.mutate(change, {
    onSuccess: () => {
      form.discard(Object.keys(change.values ?? {}))
      toast.add({ severity: 'success', summary: 'Settings saved', life: 3000 })
    },
    onError: (error) => showError(group, error),
    onSettled: () => (savingGroup.value = ''),
  })
}

function reset(group: SettingsGroup, key: string) {
  clearErrors(group)
  savingGroup.value = group.id
  update.mutate(
    { reset: [key] },
    {
      onSuccess: () => {
        form.discard([key])
        toast.add({ severity: 'success', summary: `${key} reset to its default`, life: 3000 })
      },
      onError: (error) => showError(group, error),
      onSettled: () => (savingGroup.value = ''),
    },
  )
}

function discard(group: SettingsGroup) {
  clearErrors(group)
  form.discard(group.keys)
}

function onEdit(group: SettingsGroup, key: string, value: string) {
  delete fieldError[key]
  delete groupError[group.id]
  form.setValue(key, value)
}

/** The SMTP values as they are on screen right now, for the test email. A new password is sent only if typed. */
const connection = computed(() => {
  const password = form.secretOf('SMTP_PASSWORD')
  return {
    from: form.valueOf('MAIL_FROM'),
    host: form.valueOf('SMTP_HOST'),
    port: form.valueOf('SMTP_PORT'),
    tls_mode: form.valueOf('SMTP_TLS_MODE'),
    username: form.valueOf('SMTP_USERNAME'),
    ...(password.mode === 'change' && password.value ? { password: password.value } : {}),
    ...(password.mode === 'remove' ? { password: '' } : {}),
  }
})

const saveLabel = (group: SettingsGroup) =>
  group.id === 'smtp' && form.dirtyKeys(group.keys).some((k) => k !== 'MAIL_FROM')
    ? 'Test connection and save'
    : 'Save changes'
</script>

<template>
  <PageHeader title="Settings" />

  <p v-if="settings.isPending.value" class="text-muted py-10 text-sm" role="status">
    Loading settings…
  </p>

  <div
    v-else-if="settings.isError.value"
    class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
    role="alert"
  >
    <span class="text-sm"
      ><b>Could not load the settings.</b> {{ settings.error.value?.message }}</span
    >
    <AppButton @click="() => settings.refetch()">Try again</AppButton>
  </div>

  <template v-else-if="active">
    <SettingsTabBar v-model="activeId" :tabs="tabs" />

    <section
      :id="`panel-${active.id}`"
      class="rounded-tile bg-tile p-6"
      role="tabpanel"
      :aria-labelledby="`tab-${active.id}`"
    >
      <h2 class="mb-1 text-lg font-bold tracking-tight">{{ active.title }}</h2>
      <p class="text-muted mb-4 text-sm">{{ active.subtitle }}</p>

      <SettingRow
        v-for="item in rowsOf(active)"
        :key="item.key"
        :item="item"
        :value="form.valueOf(item.key)"
        :error="fieldError[item.key]"
        :secret-mode="form.secretOf(item.key).mode"
        :secret-value="form.secretOf(item.key).value"
        @update:value="onEdit(active, item.key, $event)"
        @reset="reset(active, item.key)"
        @secret-change="form.changePassword(item.key)"
        @secret-type="form.typePassword(item.key, $event)"
        @secret-remove="form.removePassword(item.key)"
        @secret-keep="form.keepPassword(item.key)"
      />

      <TestEmailBox
        v-if="active.id === 'smtp'"
        :connection="connection"
        :default-to="form.valueOf('MAIL_FROM')"
      />

      <p
        v-if="active.id === 'server' && restartPending"
        class="bg-attn text-attn-ink rounded-inner mt-3.5 px-4 py-3.5 text-sm"
        role="status"
      >
        <b>Saved. Restart the server to apply.</b> The new values are stored and will be used the
        next time it starts.
      </p>

      <p
        v-if="groupError[active.id]"
        class="bg-bad/10 text-bad-ink rounded-inner mt-3.5 px-4 py-3.5 text-sm"
        role="alert"
      >
        {{ groupError[active.id] }}
      </p>

      <div
        v-if="editable(active)"
        class="border-hairline mt-4 flex justify-end gap-2.5 border-t pt-4"
      >
        <AppButton
          variant="secondary"
          :disabled="form.dirtyKeys(active.keys).length === 0 || savingGroup !== ''"
          @click="discard(active)"
          >Discard</AppButton
        >
        <AppButton
          :loading="savingGroup === active.id"
          :disabled="form.dirtyKeys(active.keys).length === 0"
          @click="save(active)"
        >
          {{ saveLabel(active) }}
        </AppButton>
      </div>
    </section>
  </template>
</template>
