<script setup lang="ts">
import { computed, defineAsyncComponent, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import SegmentedControl, { type SegmentOption } from '@/components/ui/SegmentedControl.vue'
import { dayLabel, RANGES, type Range } from '@/lib/dashboard'
import { formatNumber } from '@/lib/format'
import type { Stats } from '@/types/api'

// Chart.js is a big library and only this tile needs it, so it loads on demand.
const ActivityChart = defineAsyncComponent(() => import('./ActivityChart.vue'))

const props = defineProps<{ daily: Stats['daily'] }>()
const range = defineModel<Range>('range', { required: true })

const options: SegmentOption<Range>[] = RANGES.map((n) => ({ value: n, label: `${n} days` }))
const asTable = shallowRef(false)
const hasActivity = computed(() => props.daily.some((d) => d.sent > 0 || d.failed > 0))
const newestFirst = computed(() => [...props.daily].reverse())
</script>

<template>
  <section class="rounded-tile bg-tile p-6" aria-labelledby="activity-title">
    <div class="mb-3.5 flex flex-wrap items-center justify-between gap-2.5">
      <h2 id="activity-title" class="text-lg font-bold tracking-tight">Last {{ range }} days</h2>
      <div class="flex flex-wrap items-center gap-3">
        <span class="text-muted flex gap-3.5 text-xs" aria-hidden="true">
          <span class="flex items-center gap-1.5"
            ><i class="bg-ok inline-block size-2.5 rounded-sm" />Sent</span
          >
          <span class="flex items-center gap-1.5"
            ><i class="bg-bad inline-block size-2.5 rounded-sm" />Failed</span
          >
        </span>
        <SegmentedControl v-model="range" label="Time range" :options="options" />
        <button
          v-if="hasActivity"
          type="button"
          class="bg-pill hover:bg-track cursor-pointer rounded-full px-3.5 py-1.5 text-[0.78rem] font-semibold"
          :aria-pressed="asTable"
          @click="asTable = !asTable"
        >
          {{ asTable ? 'Show chart' : 'Show as table' }}
        </button>
      </div>
    </div>

    <div v-if="!hasActivity" class="text-muted py-9 text-center text-sm">
      <b class="text-ink mb-1.5 block text-base">No emails sent in the last {{ range }} days</b>
      Send a Bulk Email and its results will show up here.
      <RouterLink
        :to="{ name: 'new-bulk-email' }"
        class="bg-ink mt-4 inline-block rounded-full px-5 py-2.5 text-sm font-semibold text-white"
        >+ New Bulk Email</RouterLink
      >
    </div>
    <div v-else-if="asTable" class="max-h-56 overflow-auto">
      <table class="w-full border-collapse text-sm">
        <thead>
          <tr class="text-muted text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
            <th scope="col" class="bg-tile sticky top-0 py-2 pr-3 text-left font-semibold">Day</th>
            <th scope="col" class="bg-tile sticky top-0 px-3 py-2 text-right font-semibold">
              Sent
            </th>
            <th scope="col" class="bg-tile sticky top-0 py-2 pl-3 text-right font-semibold">
              Failed
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="day in newestFirst" :key="day.date" class="border-hairline border-t">
            <th scope="row" class="py-2 pr-3 text-left font-medium">{{ dayLabel(day.date) }}</th>
            <td class="px-3 py-2 text-right tabular-nums">{{ formatNumber(day.sent) }}</td>
            <td
              class="py-2 pl-3 text-right tabular-nums"
              :class="day.failed > 0 && 'text-bad-ink font-semibold'"
            >
              {{ formatNumber(day.failed) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <ActivityChart v-else :days="daily" />
  </section>
</template>
