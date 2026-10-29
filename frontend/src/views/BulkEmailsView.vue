<script setup lang="ts">
import { computed } from 'vue'
import { Plus } from '@lucide/vue'

import BatchTable from '@/components/batches/BatchTable.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import PaginationBar from '@/components/ui/PaginationBar.vue'
import SearchBox from '@/components/ui/SearchBox.vue'
import StatusTabs, { type TabOption } from '@/components/ui/StatusTabs.vue'
import { PAGE_SIZE, useUrlFilters } from '@/composables/useUrlFilters'
import { useBatches, useBatchStatusCounts } from '@/queries/useBatches'
import type { BatchStatus } from '@/types/api'

const STATUSES = ['pending', 'processing', 'completed'] as const satisfies readonly BatchStatus[]

const { status, q, page, offset, update } = useUrlFilters(STATUSES)
const counts = useBatchStatusCounts()

const params = computed(() => ({
  status: status.value,
  q: q.value,
  limit: PAGE_SIZE,
  offset: offset.value,
}))
const { data, isPending, isError, error, refetch, isFetching } = useBatches(params)

type Tab = (typeof STATUSES)[number] | ''
const tabs = computed<TabOption<Tab>[]>(() => [
  { value: '', label: 'All', count: counts.value[''] },
  { value: 'pending', label: 'Pending', count: counts.value.pending },
  { value: 'processing', label: 'Processing', count: counts.value.processing },
  { value: 'completed', label: 'Completed', count: counts.value.completed },
])

const filtered = computed(() => status.value !== '' || q.value !== '')
</script>

<template>
  <PageHeader title="Bulk Emails" subtitle="Every file you have uploaded">
    <template #actions>
      <RouterLink
        :to="{ name: 'new-bulk-email' }"
        class="bg-ink inline-flex items-center gap-2 rounded-full px-5.5 py-3 text-sm font-semibold text-white hover:bg-black"
      >
        <Plus class="size-4" aria-hidden="true" /> New Bulk Email
      </RouterLink>
    </template>
  </PageHeader>

  <section class="rounded-tile bg-tile p-6">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3.5">
      <StatusTabs
        label="Filter by status"
        :tabs="tabs"
        :model-value="status"
        @update:model-value="update({ status: $event })"
      />
      <SearchBox
        label="Search filename"
        placeholder="Search filename"
        :model-value="q"
        @update:model-value="update({ q: $event })"
      />
    </div>

    <p v-if="isPending" class="text-muted py-10 text-center text-sm" role="status">
      Loading Bulk Emails…
    </p>

    <div
      v-else-if="isError"
      class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
      role="alert"
    >
      <span class="text-sm"><b>Could not load Bulk Emails.</b> {{ error?.message }}</span>
      <button
        type="button"
        class="bg-ink cursor-pointer rounded-full px-4 py-2 text-sm font-semibold text-white"
        :disabled="isFetching"
        @click="() => refetch()"
      >
        Try again
      </button>
    </div>

    <template v-else-if="data">
      <BatchTable v-if="data.items.length" :items="data.items" />
      <p v-else class="text-muted py-10 text-center text-sm">
        {{
          filtered
            ? 'No Bulk Emails match these filters.'
            : 'No Bulk Emails yet. Upload a spreadsheet to send your first one.'
        }}
      </p>
      <PaginationBar
        :page="page"
        :page-size="PAGE_SIZE"
        :total="data.total"
        @update:page="update({ page: $event })"
      />
    </template>
  </section>
</template>
