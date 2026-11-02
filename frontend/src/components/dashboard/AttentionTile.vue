<script setup lang="ts">
import { attentionReasons } from '@/lib/dashboard'
import type { AttentionBatch } from '@/types/api'

defineProps<{ items: AttentionBatch[] }>()
</script>

<template>
  <section
    class="rounded-tile p-6"
    :class="items.length ? 'bg-attn text-attn-ink' : 'bg-ok/10 text-ok-ink'"
    aria-labelledby="attention-title"
  >
    <h2
      id="attention-title"
      class="mb-3.5 flex items-center justify-between text-lg font-bold tracking-tight"
    >
      Needs attention
      <span
        v-if="items.length"
        class="text-ink rounded-full bg-white px-2.5 py-0.5 text-xs font-semibold"
        >{{ items.length }}</span
      >
    </h2>

    <p v-if="items.length === 0" class="flex items-center gap-3 text-[0.9375rem] font-semibold">
      <span class="grid size-8.5 place-items-center rounded-full bg-white" aria-hidden="true"
        >✓</span
      >
      Nothing needs attention
    </p>

    <ul v-else class="list-none">
      <li
        v-for="item in items"
        :key="item.batch_id"
        class="flex items-center justify-between gap-3 border-t border-black/10 py-3 first:border-t-0 first:pt-0"
      >
        <div class="min-w-0">
          <b class="text-user block truncate text-sm">{{
            item.filename || `Batch #${item.batch_id}`
          }}</b>
          <span class="mt-0.5 block text-[0.8125rem]">{{
            attentionReasons(item).join(' · ')
          }}</span>
        </div>
        <RouterLink
          :to="{ name: 'bulk-email-detail', params: { id: item.batch_id } }"
          class="bg-ink shrink-0 rounded-full px-3.5 py-1.5 text-[0.8125rem] font-semibold text-white hover:bg-black"
          :aria-label="`Review ${item.filename || 'batch ' + item.batch_id}`"
        >
          Review
        </RouterLink>
      </li>
    </ul>
  </section>
</template>
