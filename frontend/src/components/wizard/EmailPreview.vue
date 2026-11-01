<script setup lang="ts">
import { ChevronLeft, ChevronRight } from '@lucide/vue'

defineProps<{ to: string; subject: string; body: string; position: number; total: number }>()
const emit = defineEmits<{ previous: []; next: [] }>()
</script>

<template>
  <div class="rounded-inner bg-note p-4.5">
    <div class="text-muted mb-2.5 flex items-center justify-between gap-2.5 text-[0.8125rem]">
      <span>Preview for recipient {{ position }} of {{ total }}</span>
      <div v-if="total > 1" class="flex gap-1.5">
        <button
          type="button"
          class="bg-track hover:bg-hover grid size-7 cursor-pointer place-items-center rounded-full"
          aria-label="Previous recipient"
          @click="emit('previous')"
        >
          <ChevronLeft class="size-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="bg-track hover:bg-hover grid size-7 cursor-pointer place-items-center rounded-full"
          aria-label="Next recipient"
          @click="emit('next')"
        >
          <ChevronRight class="size-4" aria-hidden="true" />
        </button>
      </div>
    </div>
    <p class="text-muted text-user mb-1 text-[0.8125rem]">To: {{ to }}</p>
    <p class="text-user mb-3 text-base font-bold">{{ subject }}</p>
    <!-- Plain text, never HTML: the email is sent as text, so the preview shows exactly that. -->
    <p class="text-user text-[0.9rem] whitespace-pre-wrap">{{ body }}</p>
  </div>
</template>
