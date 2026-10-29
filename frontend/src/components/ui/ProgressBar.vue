<script setup lang="ts">
import { computed } from 'vue'

import { percent } from '@/lib/format'

const props = defineProps<{ value: number; total: number; label: string }>()
const pct = computed(() => percent(props.value, props.total))
</script>

<template>
  <div
    class="bg-track h-1.5 min-w-22 overflow-hidden rounded-full"
    role="progressbar"
    :aria-label="label"
    aria-valuemin="0"
    :aria-valuemax="total"
    :aria-valuenow="value"
  >
    <div class="bg-ink h-full rounded-full" :style="{ width: `${pct}%` }" />
  </div>
</template>
