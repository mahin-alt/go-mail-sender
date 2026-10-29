<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { ChevronLeft, RotateCw } from '@lucide/vue'
import { useRoute } from 'vue-router'

import JobDrawer from '@/components/batches/JobDrawer.vue'
import JobsTable from '@/components/batches/JobsTable.vue'
import RetryConfirmDialog from '@/components/batches/RetryConfirmDialog.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import StatusBadge from '@/components/status/StatusBadge.vue'
import KpiTile from '@/components/ui/KpiTile.vue'
import PaginationBar from '@/components/ui/PaginationBar.vue'
import ProgressBar from '@/components/ui/ProgressBar.vue'
import SearchBox from '@/components/ui/SearchBox.vue'
import StatusTabs, { type TabOption } from '@/components/ui/StatusTabs.vue'
import { ApiError } from '@/api/errors'
import { PAGE_SIZE, useUrlFilters } from '@/composables/useUrlFilters'
import { useRetryFeedback } from '@/composables/useRetryFeedback'
import { exactTime, formatNumber, percent, timeAgo } from '@/lib/format'
import { useBatch, useJobs, useRetryBatch, useRetryJob } from '@/queries/useBatches'
import type { Job, JobStatus } from '@/types/api'

const JOB_STATUSES = [
  'sent',
  'failed',
  'invalid',
  'pending',
] as const satisfies readonly JobStatus[]

const route = useRoute()
const id = computed(() => Number(route.params.id))

const { status, q, page, offset, update } = useUrlFilters(JOB_STATUSES)
const batch = useBatch(id)
const jobParams = computed(() => ({
  status: status.value,
  q: q.value,
  limit: PAGE_SIZE,
  offset: offset.value,
}))
const jobs = useJobs(id, jobParams)

const retryBatch = useRetryBatch()
const retryJob = useRetryJob()
const feedback = useRetryFeedback()

const progress = computed(() => batch.data.value)
type Tab = (typeof JOB_STATUSES)[number] | ''
const tabs = computed<TabOption<Tab>[]>(() => {
  const p = progress.value
  return [
    { value: '', label: 'All', count: p?.total },
    { value: 'sent', label: 'Sent', count: p?.sent },
    { value: 'failed', label: 'Failed', count: p?.failed },
    { value: 'invalid', label: 'Invalid', count: p?.invalid },
    { value: 'pending', label: 'Pending', count: p?.pending },
  ]
})

const confirmOpen = shallowRef(false)
const selected = shallowRef<Job | null>(null)
const drawerOpen = shallowRef(false)

function openJob(job: Job) {
  selected.value = job
  drawerOpen.value = true
}

function confirmRetryBatch() {
  retryBatch.mutate(id.value, {
    onSuccess: (result) => {
      confirmOpen.value = false
      feedback.requeued(result.retried)
    },
    onError: (error) => feedback.failed(error),
  })
}

function retryOne(job: Job) {
  retryJob.mutate(job.id, {
    onSuccess: (result) => {
      drawerOpen.value = false
      feedback.requeued(result.retried)
    },
    onError: (error) => feedback.failed(error),
  })
}

const notFound = computed(
  () => batch.error.value instanceof ApiError && batch.error.value.status === 404,
)
const retryingJobId = computed(() => (retryJob.isPending.value ? retryJob.variables.value : null))
</script>

<template>
  <RouterLink
    :to="{ name: 'bulk-emails' }"
    class="text-muted hover:text-ink mb-2 inline-flex items-center gap-1 text-[0.8125rem] font-semibold"
  >
    <ChevronLeft class="size-4" aria-hidden="true" /> Bulk Emails
  </RouterLink>

  <p v-if="batch.isPending.value" class="text-muted py-10 text-sm" role="status">Loading…</p>

  <div
    v-else-if="batch.isError.value"
    class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
    role="alert"
  >
    <span class="text-sm">
      <b>{{ notFound ? 'This Bulk Email does not exist.' : batch.error.value?.message }}</b>
    </span>
    <RouterLink
      :to="{ name: 'bulk-emails' }"
      class="bg-ink rounded-full px-4 py-2 text-sm font-semibold text-white"
      >Back to Bulk Emails</RouterLink
    >
  </div>

  <template v-else-if="progress">
    <PageHeader
      :title="progress.filename || `Batch #${progress.batch_id}`"
      :subtitle="`Batch #${progress.batch_id} · ${formatNumber(progress.total)} emails`"
    >
      <template #actions>
        <StatusBadge :status="progress.status" :failed="progress.failed" />
        <button
          v-if="progress.failed > 0"
          type="button"
          class="bg-ink inline-flex cursor-pointer items-center gap-2 rounded-full px-5.5 py-3 text-sm font-semibold text-white hover:bg-black"
          :aria-label="`Retry ${progress.failed} failed ${progress.failed === 1 ? 'email' : 'emails'}`"
          @click="confirmOpen = true"
        >
          <RotateCw class="size-4" aria-hidden="true" /> Retry
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 nav:grid-cols-4">
      <KpiTile
        tone="ink"
        label="Sent"
        :value="progress.sent"
        :caption="`${percent(progress.sent, progress.total)}% of ${formatNumber(progress.total)}`"
      />
      <KpiTile
        tone="danger"
        label="Failed"
        :value="progress.failed"
        :caption="progress.failed > 0 ? 'can be retried' : 'none'"
      />
      <KpiTile
        label="Pending"
        :value="progress.pending"
        :caption="progress.processing > 0 ? `${progress.processing} sending now` : 'none waiting'"
      />
      <KpiTile label="Invalid" :value="progress.invalid" caption="skipped rows" />
    </div>

    <section class="rounded-tile bg-tile mt-3.5 flex items-center gap-4 px-6 py-5">
      <h2 class="text-lg font-bold tracking-tight">Progress</h2>
      <ProgressBar
        class="flex-1"
        :value="progress.sent"
        :total="progress.total"
        label="Emails sent"
      />
      <span class="text-sm font-bold whitespace-nowrap tabular-nums"
        >{{ formatNumber(progress.sent) }} / {{ formatNumber(progress.total) }}</span
      >
    </section>

    <section class="rounded-tile bg-tile mt-3.5 p-6">
      <h2 class="mb-3.5 text-lg font-bold tracking-tight">Emails</h2>
      <div class="mb-3 flex flex-wrap items-center justify-between gap-3.5">
        <StatusTabs
          label="Filter by status"
          :tabs="tabs"
          :model-value="status"
          @update:model-value="update({ status: $event })"
        />
        <SearchBox
          label="Search name or email"
          placeholder="Search name or email"
          :model-value="q"
          @update:model-value="update({ q: $event })"
        />
      </div>

      <p v-if="jobs.isPending.value" class="text-muted py-10 text-center text-sm" role="status">
        Loading emails…
      </p>
      <div
        v-else-if="jobs.isError.value"
        class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
        role="alert"
      >
        <span class="text-sm"><b>Could not load emails.</b> {{ jobs.error.value?.message }}</span>
        <button
          type="button"
          class="bg-ink cursor-pointer rounded-full px-4 py-2 text-sm font-semibold text-white"
          @click="() => jobs.refetch()"
        >
          Try again
        </button>
      </div>
      <template v-else-if="jobs.data.value">
        <JobsTable
          v-if="jobs.data.value.items.length"
          :jobs="jobs.data.value.items"
          :retrying-id="retryingJobId"
          @select="openJob"
          @retry="retryOne"
        />
        <p v-else class="text-muted py-10 text-center text-sm">No emails match these filters.</p>
        <PaginationBar
          :page="page"
          :page-size="PAGE_SIZE"
          :total="jobs.data.value.total"
          @update:page="update({ page: $event })"
        />
      </template>
    </section>

    <p class="text-muted mt-3 text-xs" :title="exactTime(undefined)">
      Updates every few seconds · last checked
      {{ timeAgo(new Date(batch.dataUpdatedAt.value).toISOString()) }}
    </p>

    <JobDrawer
      v-model:visible="drawerOpen"
      :job="selected"
      :retrying="retryJob.isPending.value"
      @retry="retryOne"
    />
    <RetryConfirmDialog
      v-model:visible="confirmOpen"
      :count="progress.failed"
      :busy="retryBatch.isPending.value"
      @confirm="confirmRetryBatch"
    />
  </template>
</template>
