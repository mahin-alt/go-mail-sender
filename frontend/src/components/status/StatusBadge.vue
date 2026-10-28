<script setup lang="ts">
import { computed } from 'vue'
import Tag from 'primevue/tag'

import { describeStatus, type Status } from './statusMeta'

const props = defineProps<{
  status: Status
  /** Attempts so far; with `maxAttempts` turns a pending job into "Retrying 2 of 4". */
  attempts?: number
  maxAttempts?: number
}>()

const meta = computed(() => describeStatus(props.status, props.attempts, props.maxAttempts))
</script>

<template>
  <Tag :severity="meta.severity" :data-status="status">
    <template #icon>
      <component
        :is="meta.icon"
        :size="14"
        aria-hidden="true"
        :class="{ 'motion-safe:animate-spin': meta.spin }"
      />
    </template>
    {{ meta.label }}
  </Tag>
</template>
