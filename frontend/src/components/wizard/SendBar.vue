<script setup lang="ts">
import { formatNumber } from '@/lib/format'

defineProps<{ ready: boolean; valid: number; skipped: number; blocker: string }>()
const emit = defineEmits<{ send: [] }>()
</script>

<template>
  <div
    class="bg-ink sticky bottom-3 z-10 mt-3.5 flex flex-wrap items-center justify-between gap-3.5 rounded-full py-3 pr-3 pl-6 text-white shadow-xl"
  >
    <p class="text-sm" role="status">
      <template v-if="ready">
        <b>{{ formatNumber(valid) }}</b> will be sent · <b>{{ formatNumber(skipped) }}</b> skipped
        <span v-if="blocker" class="text-white/70"> · {{ blocker }}</span>
      </template>
      <template v-else>Upload a file to begin</template>
    </p>
    <button
      type="button"
      class="text-ink cursor-pointer rounded-full bg-white px-5 py-3 text-sm font-semibold hover:bg-white/90 disabled:cursor-default disabled:opacity-40"
      :disabled="blocker !== ''"
      :title="blocker"
      @click="emit('send')"
    >
      Send to {{ formatNumber(valid) }} {{ valid === 1 ? 'recipient' : 'recipients' }}
    </button>
  </div>
</template>
