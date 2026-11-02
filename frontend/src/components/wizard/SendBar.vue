<script setup lang="ts">
import { formatNumber } from '@/lib/format'

defineProps<{ valid: number; blocker: string }>()
const emit = defineEmits<{ send: [] }>()
</script>

<template>
  <div class="sticky bottom-3 z-10 mt-3.5 flex justify-end">
    <div class="bg-ink inline-flex rounded-full p-1.5 shadow-xl">
      <button
        type="button"
        class="text-ink cursor-pointer rounded-full bg-white px-7 py-3 text-sm font-semibold hover:bg-white/90 disabled:cursor-default disabled:opacity-40"
        :disabled="blocker !== ''"
        :title="blocker"
        :aria-label="
          valid > 0
            ? `Send to ${formatNumber(valid)} ${valid === 1 ? 'recipient' : 'recipients'}`
            : 'Send'
        "
        @click="emit('send')"
      >
        Send
      </button>
    </div>
    <!-- Why Send is off, for people who cannot see the tooltip. -->
    <span class="sr-only" role="status">{{ blocker }}</span>
  </div>
</template>
