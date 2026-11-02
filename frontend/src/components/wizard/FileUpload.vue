<script setup lang="ts">
import { shallowRef, useTemplateRef } from 'vue'

import { formatNumber } from '@/lib/format'

defineProps<{
  file: File | null
  /** True while the server is checking the file. */
  checking: boolean
  /** Problem with the file or the server's answer. */
  error: string
}>()
const emit = defineEmits<{ choose: [file: File]; clear: [] }>()

const input = useTemplateRef<HTMLInputElement>('input')
const dragging = shallowRef(false)

function onPick(event: Event) {
  const picked = (event.target as HTMLInputElement).files?.[0]
  if (picked) emit('choose', picked)
  if (input.value) input.value.value = '' // allows choosing the same file again after a fix
}

function onDrop(event: DragEvent) {
  dragging.value = false
  const dropped = event.dataTransfer?.files[0]
  if (dropped) emit('choose', dropped)
}

function size(bytes: number) {
  return bytes < 1024 * 1024
    ? `${formatNumber(Math.max(1, Math.round(bytes / 1024)))} KB`
    : `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <div>
    <p
      v-if="error"
      class="rounded-inner bg-bad/10 text-bad-ink mb-3.5 px-4 py-3 text-sm"
      role="alert"
    >
      {{ error }}
    </p>

    <div v-if="file" class="rounded-inner bg-note flex items-center gap-3.5 px-4.5 py-3.5">
      <span
        class="bg-ink grid size-10.5 shrink-0 place-items-center rounded-xl text-xs font-bold text-white"
        aria-hidden="true"
        >XLSX</span
      >
      <div class="min-w-0">
        <b class="block truncate text-sm">{{ file.name }}</b>
        <span class="text-muted text-[0.8125rem]">{{ size(file.size) }}</span>
      </div>
      <button
        v-if="!checking"
        type="button"
        class="bg-track hover:bg-hover ml-auto cursor-pointer rounded-full px-3.5 py-2 text-[0.8125rem] font-semibold"
        @click="emit('clear')"
      >
        Remove
      </button>
    </div>
    <p
      v-if="file && checking"
      class="bg-note rounded-inner mt-3.5 flex items-center gap-2.5 px-4 py-3.5 text-sm"
      role="status"
    >
      <span
        class="border-track border-t-ink size-4 animate-spin rounded-full border-[2.5px] motion-reduce:animate-none"
        aria-hidden="true"
      />
      Checking every row. Nothing is sent yet.
    </p>
    <div
      v-if="!file"
      class="rounded-tile bg-note border-2 border-dashed px-5 py-10 text-center"
      :class="dragging ? 'border-ink' : 'border-track'"
      @dragover.prevent="dragging = true"
      @dragleave="dragging = false"
      @drop.prevent="onDrop"
    >
      <b class="mb-1.5 block text-base">Drop your .xlsx file here</b>
      <p class="text-muted mb-4 text-sm">
        First sheet, with a header row that has Name and Email. Up to 10 MB.
      </p>
      <button
        type="button"
        class="bg-ink cursor-pointer rounded-full px-5 py-3 text-sm font-semibold text-white hover:bg-black"
        @click="input?.click()"
      >
        Choose file
      </button>
    </div>
    <input
      ref="input"
      type="file"
      accept=".xlsx"
      class="sr-only"
      tabindex="-1"
      aria-label="Spreadsheet file"
      @change="onPick"
    />
  </div>
</template>
