<script setup lang="ts">
export interface SettingsTab {
  id: string
  label: string
  /** Shows an orange dot when this tab has unsaved changes. */
  dirty: boolean
}

const props = defineProps<{ tabs: SettingsTab[] }>()
const active = defineModel<string>({ required: true })

/** Arrow keys move between tabs, as people expect from a tab bar. */
function onKey(event: KeyboardEvent, index: number) {
  const step = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
  if (!step) return
  event.preventDefault()
  const next = props.tabs[(index + step + props.tabs.length) % props.tabs.length]
  if (!next) return
  active.value = next.id
  document.getElementById(`tab-${next.id}`)?.focus()
}
</script>

<template>
  <div class="mb-3.5 flex flex-wrap gap-1.5" role="tablist" aria-label="Settings groups">
    <button
      v-for="(tab, index) in tabs"
      :id="`tab-${tab.id}`"
      :key="tab.id"
      type="button"
      role="tab"
      class="cursor-pointer rounded-full px-4 py-2.5 text-sm font-semibold"
      :class="active === tab.id ? 'bg-ink text-white' : 'bg-tile text-ink hover:bg-white/70'"
      :aria-selected="active === tab.id"
      :aria-controls="`panel-${tab.id}`"
      :tabindex="active === tab.id ? 0 : -1"
      @click="active = tab.id"
      @keydown="onKey($event, index)"
    >
      {{ tab.label }}
      <span
        v-if="tab.dirty"
        class="bg-warn ml-2 inline-block size-2 rounded-full align-middle"
        aria-hidden="true"
      />
      <span v-if="tab.dirty" class="sr-only">(unsaved changes)</span>
    </button>
  </div>
</template>
