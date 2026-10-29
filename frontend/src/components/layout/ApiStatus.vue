<script setup lang="ts">
import { computed } from 'vue'

import { useApiHealth } from '@/queries/useApiHealth'

defineProps<{ showLabel?: boolean }>()

const { isPending, isError } = useApiHealth()

const state = computed(() => {
  if (isPending.value) return { label: 'Checking API', dot: 'bg-muted' }
  if (isError.value) return { label: 'API offline', dot: 'bg-bad' }
  return { label: 'API online', dot: 'bg-ok' }
})
</script>

<template>
  <div
    class="text-muted flex h-8 items-center gap-3 px-4 text-xs font-medium whitespace-nowrap"
    role="status"
  >
    <span class="size-2.5 shrink-0 rounded-full" :class="state.dot" aria-hidden="true" />
    <span
      class="transition-opacity duration-150 motion-reduce:transition-none"
      :class="showLabel ? 'opacity-100' : 'opacity-0'"
      >{{ state.label }}</span
    >
  </div>
</template>
