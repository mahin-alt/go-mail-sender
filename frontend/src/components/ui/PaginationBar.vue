<script setup lang="ts">
import { computed } from 'vue'

import { formatNumber } from '@/lib/format'

const props = defineProps<{ page: number; pageSize: number; total: number }>()
const emit = defineEmits<{ 'update:page': [page: number] }>()

const first = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1))
const last = computed(() => Math.min(props.page * props.pageSize, props.total))
const hasPrevious = computed(() => props.page > 1)
const hasNext = computed(() => props.page * props.pageSize < props.total)
</script>

<template>
  <div
    class="border-hairline text-muted mt-1 flex items-center justify-between border-t pt-3.5 text-[0.8125rem]"
  >
    <span>{{ formatNumber(first) }}–{{ formatNumber(last) }} of {{ formatNumber(total) }}</span>
    <div class="flex gap-2">
      <button
        type="button"
        class="bg-pill text-ink hover:bg-track cursor-pointer rounded-full px-3.5 py-2 font-semibold disabled:cursor-default disabled:opacity-45 disabled:hover:bg-pill"
        :disabled="!hasPrevious"
        @click="emit('update:page', page - 1)"
      >
        Previous
      </button>
      <button
        type="button"
        class="bg-pill text-ink hover:bg-track cursor-pointer rounded-full px-3.5 py-2 font-semibold disabled:cursor-default disabled:opacity-45 disabled:hover:bg-pill"
        :disabled="!hasNext"
        @click="emit('update:page', page + 1)"
      >
        Next
      </button>
    </div>
  </div>
</template>
