<script setup lang="ts">
import { computed } from 'vue'

import { useApiHealth, type ApiConnection } from '@/queries/useApiHealth'

defineProps<{
  /** In the narrow icon rail the text is visually hidden (still read by screen readers). */
  rail?: boolean
}>()

const { connection } = useApiHealth()

const TEXT: Record<ApiConnection, string> = {
  checking: 'Checking API…',
  connected: 'API connected',
  unreachable: 'API unreachable',
}
const DOT: Record<ApiConnection, string> = {
  checking: 'bg-surface-400',
  connected: 'bg-green-600',
  unreachable: 'bg-red-600',
}

const text = computed(() => TEXT[connection.value])
</script>

<template>
  <!-- The dot is decorative: the text carries the meaning. -->
  <p role="status" class="flex items-center gap-2 text-xs text-surface-600" :title="text">
    <span class="size-2 shrink-0 rounded-full" :class="DOT[connection]" aria-hidden="true" />
    <span :class="{ 'sr-only xl:not-sr-only': rail }">{{ text }}</span>
  </p>
</template>
