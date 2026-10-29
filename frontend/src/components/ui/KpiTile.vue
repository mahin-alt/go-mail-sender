<script setup lang="ts">
import { formatNumber } from '@/lib/format'

withDefaults(
  defineProps<{
    label: string
    value: number
    caption?: string
    /** `ink` is the black tile, `danger` colours the number red, `plain` is a white tile. */
    tone?: 'plain' | 'ink' | 'danger'
  }>(),
  { caption: '', tone: 'plain' },
)
</script>

<template>
  <div
    class="rounded-tile flex min-h-36 flex-col justify-between p-6"
    :class="tone === 'ink' ? 'bg-ink text-white' : 'bg-tile'"
  >
    <span class="text-sm font-medium" :class="tone === 'ink' ? 'text-white/75' : 'text-muted'">{{
      label
    }}</span>
    <strong
      class="text-5xl leading-none font-bold tracking-tighter"
      :class="tone === 'danger' ? 'text-bad' : ''"
      >{{ formatNumber(value) }}</strong
    >
    <span class="text-[0.8125rem]" :class="tone === 'ink' ? 'text-white/75' : 'text-muted'"
      >{{ caption }}&nbsp;</span
    >
  </div>
</template>
