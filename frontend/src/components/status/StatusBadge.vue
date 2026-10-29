<script setup lang="ts">
import { computed } from 'vue'

import { describeStatus, type Status, type StatusContext, type Tone } from './statusMeta'

const props = defineProps<
  {
    status: Status
  } & StatusContext
>()

const meta = computed(() => describeStatus(props.status, props))

const toneClass: Record<Tone, string> = {
  ok: 'text-ok-ink',
  bad: 'text-bad-ink',
  warn: 'text-warn-ink',
  active: 'text-ink',
  quiet: 'text-muted',
}
</script>

<template>
  <span
    class="bg-pill inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-[0.8125rem] font-semibold whitespace-nowrap"
    :class="toneClass[meta.tone]"
  >
    <component
      :is="meta.icon"
      class="size-3.5 shrink-0"
      :class="meta.spin && 'animate-spin motion-reduce:animate-none'"
      aria-hidden="true"
    />
    {{ meta.label }}
  </span>
</template>
