<script setup lang="ts">
import { computed, shallowRef } from 'vue'

import AttentionTile from '@/components/dashboard/AttentionTile.vue'
import ActivityTile from '@/components/dashboard/ActivityTile.vue'
import BatchTable from '@/components/batches/BatchTable.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import AppButton from '@/components/ui/AppButton.vue'
import KpiTile from '@/components/ui/KpiTile.vue'
import { inQueue, queueCaption, timeZone, todayLabel, type Range } from '@/lib/dashboard'
import { formatNumber } from '@/lib/format'
import { useBatches } from '@/queries/useBatches'
import { useStats } from '@/queries/useStats'

const RECENT = 5

const range = shallowRef<Range>(14)
const stats = useStats(range)
const recent = useBatches({ type: 'bulk_send', status: '', q: '', limit: RECENT, offset: 0 })

const tz = timeZone()
const subtitle = `${todayLabel(new Date(), tz)} · ${tz}`
const data = computed(() => stats.data.value)
</script>

<template>
  <PageHeader title="Dashboard" :subtitle="subtitle" />

  <p v-if="stats.isPending.value" class="text-muted py-10 text-sm" role="status">
    Loading the Dashboard…
  </p>

  <div
    v-else-if="stats.isError.value && !data"
    class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
    role="alert"
  >
    <span class="text-sm"><b>Could not load the numbers.</b> {{ stats.error.value?.message }}</span>
    <AppButton @click="() => stats.refetch()">Try again</AppButton>
  </div>

  <template v-else-if="data">
    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 nav:grid-cols-4">
      <KpiTile
        tone="ink"
        label="Sent today"
        :value="data.today.sent"
        :caption="data.today.sent ? 'delivered since midnight' : 'nothing sent yet today'"
      />
      <KpiTile
        tone="danger"
        label="Failed today"
        :value="data.today.failed"
        :caption="data.today.failed ? 'could not be delivered' : 'all good'"
      />
      <KpiTile
        label="In the queue"
        :value="inQueue(data.queue)"
        :caption="queueCaption(data.queue)"
      />
      <KpiTile
        label="Active Bulk Emails"
        :value="data.active_batches"
        :caption="
          data.active_batches
            ? `${formatNumber(data.active_batches)} not finished yet`
            : 'nothing sending'
        "
      />
    </div>

    <div class="mt-3.5 grid grid-cols-1 gap-3.5 nav:grid-cols-12">
      <ActivityTile v-model:range="range" :daily="data.daily" class="nav:col-span-8" />
      <AttentionTile :items="data.needs_attention" class="nav:col-span-4" />
    </div>

    <section class="rounded-tile bg-tile mt-3.5 p-6" aria-labelledby="recent-title">
      <div class="mb-3.5 flex items-center justify-between">
        <h2 id="recent-title" class="text-lg font-bold tracking-tight">Recent Bulk Emails</h2>
        <RouterLink
          :to="{ name: 'bulk-emails' }"
          class="text-muted hover:text-ink text-[0.8125rem] font-semibold"
          >View all</RouterLink
        >
      </div>
      <p v-if="recent.isPending.value" class="text-muted py-6 text-sm" role="status">Loading…</p>
      <p v-else-if="recent.isError.value" class="text-bad-ink py-6 text-sm" role="alert">
        Could not load the recent Bulk Emails. {{ recent.error.value?.message }}
      </p>
      <BatchTable v-else-if="recent.data.value?.items.length" :items="recent.data.value.items" />
      <div v-else class="text-muted py-9 text-center text-sm">
        <b class="text-ink mb-1.5 block text-base">No Bulk Emails yet</b>
        Upload a spreadsheet to send your first one.
        <RouterLink
          :to="{ name: 'new-bulk-email' }"
          class="bg-ink mt-4 inline-block rounded-full px-5 py-2.5 text-sm font-semibold text-white"
          >+ New Bulk Email</RouterLink
        >
      </div>
    </section>

    <p class="text-muted mt-3 text-xs">Updates every few seconds</p>
  </template>
</template>
