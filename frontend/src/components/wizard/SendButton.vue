<script setup lang="ts">
import AppButton from '@/components/ui/AppButton.vue'
import { formatNumber } from '@/lib/format'

defineProps<{ valid: number; blocker: string }>()
const emit = defineEmits<{ send: [] }>()
</script>

<template>
  <!-- Same button as Save on the Settings page, at the end of the page (not stuck to the screen edge). -->
  <div class="mt-3.5 flex justify-end">
    <AppButton
      size="lg"
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
    </AppButton>
    <!-- Why Send is off, for people who cannot see the tooltip. -->
    <span class="sr-only" role="status">{{ blocker }}</span>
  </div>
</template>
