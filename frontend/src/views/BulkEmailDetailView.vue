<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { Ban, ChevronLeft, RotateCw, Send } from '@lucide/vue'
import { useRoute } from 'vue-router'

import CancelConfirmDialog from '@/components/batches/CancelConfirmDialog.vue'
import JobDrawer from '@/components/batches/JobDrawer.vue'
import JobsTable from '@/components/batches/JobsTable.vue'
import RetryConfirmDialog from '@/components/batches/RetryConfirmDialog.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import StatusBadge from '@/components/status/StatusBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import KpiTile from '@/components/ui/KpiTile.vue'
import ProgressBar from '@/components/ui/ProgressBar.vue'
import SearchBox from '@/components/ui/SearchBox.vue'
import StatusTabs, { type TabOption } from '@/components/ui/StatusTabs.vue'
import { ApiError } from '@/api/errors'
import { useUrlFilters } from '@/composables/useUrlFilters'
import { useRetryFeedback } from '@/composables/useRetryFeedback'
import { exactTime, formatNumber, percent, timeAgo } from '@/lib/format'
import {
  useBatch,
  useCancelBatch,
  useEmails,
  useRetryBatch,
  useRetryJob,
} from '@/queries/useBatches'
import type { Email, JobStatus } from '@/types/api'

const JOB_STATUSES = [
  'sent',
  'failed',
  'pending',
  'processing',
  'cancelled',
] as const satisfies readonly JobStatus[]

const route = useRoute()
const id = computed(() => Number(route.params.id))

const { status, q, update } = useUrlFilters(JOB_STATUSES)
const batch = useBatch(id)
const emailParams = computed(() => ({ status: status.value, q: q.value }))

const retryBatch = useRetryBatch()
const retryJob = useRetryJob()
const cancelBatch = useCancelBatch()
const feedback = useRetryFeedback()

const progress = computed(() => batch.data.value)
/** Rows that get an email: valid + unverified. */
const sendable = computed(() =>
  progress.value ? progress.value.valid_count + progress.value.unverified_count : 0,
)
/** Not sent yet (still importing, or waiting for Send): there are no emails to list. */
const notSent = computed(() =>
  ['importing', 'import_failed', 'draft'].includes(progress.value?.status ?? ''),
)
// Emails exist only once the batch was sent; do not ask for them before.
const emails = useEmails(id, emailParams, () => !!progress.value && !notSent.value)
const emailRows = computed(() => emails.data.value?.pages.flatMap((p) => p.items) ?? [])
type Tab = (typeof JOB_STATUSES)[number] | ''
const tabs = computed<TabOption<Tab>[]>(() => {
  const p = progress.value
  return [
    { value: '', label: 'All' },
    { value: 'sent', label: 'Sent', count: p?.sent_count },
    { value: 'failed', label: 'Failed', count: p?.failed_count },
    { value: 'pending', label: 'Pending', count: p?.pending_count },
    // Only batches that were stopped have cancelled emails; keep the tab out of the way otherwise.
    ...(p && p.cancelled_count > 0
      ? [{ value: 'cancelled' as const, label: 'Cancelled', count: p.cancelled_count }]
      : []),
  ]
})

/** A batch can be stopped while emails are being prepared or sent. */
const canCancel = computed(() =>
  ['generating_jobs', 'queued', 'sending'].includes(progress.value?.status ?? ''),
)
const cancelOpen = shallowRef(false)

function confirmCancel() {
  cancelBatch.mutate(id.value, {
    onSuccess: (result) => {
      cancelOpen.value = false
      feedback.cancelled(result.cancelled)
    },
    onError: (error) => {
      cancelOpen.value = false
      feedback.failed(error, 'Could not cancel')
    },
  })
}

const confirmOpen = shallowRef(false)
const selected = shallowRef<Email | null>(null)
const drawerOpen = shallowRef(false)

function openJob(job: Email) {
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

function retryOne(job: Email) {
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
      :title="progress.filename || `Batch #${progress.id}`"
      :subtitle="`Batch #${progress.id} · ${formatNumber(sendable)} emails`"
    >
      <template #actions>
        <StatusBadge :status="progress.status" />
        <button
          v-if="progress.failed_count > 0"
          type="button"
          class="bg-ink inline-flex cursor-pointer items-center gap-2 rounded-full px-5.5 py-3 text-sm font-semibold text-white hover:bg-black"
          :aria-label="`Retry ${progress.failed_count} failed ${progress.failed_count === 1 ? 'email' : 'emails'}`"
          @click="confirmOpen = true"
        >
          <RotateCw class="size-4" aria-hidden="true" /> Retry
        </button>
        <button
          v-if="canCancel"
          type="button"
          class="bg-track hover:bg-hover text-bad-ink inline-flex cursor-pointer items-center gap-2 rounded-full px-5.5 py-3 text-sm font-semibold"
          :aria-label="`Cancel this Bulk Email (${progress.pending_count} unsent)`"
          @click="cancelOpen = true"
        >
          <Ban class="size-4" aria-hidden="true" /> Cancel
        </button>
      </template>
    </PageHeader>

    <section
      v-if="notSent"
      class="rounded-tile bg-tile flex flex-wrap items-center gap-4 p-6"
      aria-labelledby="not-sent"
    >
      <div class="min-w-0 flex-1">
        <h2 id="not-sent" class="text-lg font-bold tracking-tight">
          {{
            progress.status === 'importing'
              ? 'Checking the file'
              : progress.status === 'import_failed'
                ? 'The file could not be checked'
                : 'Not sent yet'
          }}
        </h2>
        <p class="text-muted mt-1 text-sm">
          <template v-if="progress.status === 'importing'">
            {{ formatNumber(progress.processed_rows) }} rows read
          </template>
          <template v-else-if="progress.status === 'import_failed'">
            {{ progress.import_error }}
          </template>
          <template v-else>
            {{ formatNumber(sendable) }} valid ·
            {{ formatNumber(progress.invalid_count) }} invalid
          </template>
        </p>
      </div>
      <RouterLink
        v-if="progress.status === 'draft'"
        :to="{ name: 'new-bulk-email', query: { batch: String(progress.id) } }"
        class="bg-ink inline-flex items-center gap-2 rounded-full px-5.5 py-3 text-sm font-semibold text-white hover:bg-black"
      >
        <Send class="size-4" aria-hidden="true" /> Write and send
      </RouterLink>
    </section>

    <template v-else>
      <div class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 nav:grid-cols-4">
        <KpiTile
          tone="ink"
          label="Sent"
          :value="progress.sent_count"
          :caption="`${percent(progress.sent_count, sendable)}% of ${formatNumber(sendable)}`"
        />
        <KpiTile
          tone="danger"
          label="Failed"
          :value="progress.failed_count"
          :caption="progress.failed_count > 0 ? 'can be retried' : 'none'"
        />
        <KpiTile
          v-if="progress.status === 'cancelled'"
          label="Cancelled"
          :value="progress.cancelled_count"
          caption="stopped before sending"
        />
        <KpiTile
          v-else
          label="Pending"
          :value="progress.pending_count"
          :caption="
            progress.status === 'generating_jobs'
              ? 'preparing emails'
              : progress.processing_count > 0
                ? `${progress.processing_count} sending now`
                : 'none waiting'
          "
        />
        <KpiTile label="Invalid" :value="progress.invalid_count" caption="skipped rows" />
      </div>

      <section class="rounded-tile bg-tile mt-3.5 flex items-center gap-4 px-6 py-5">
        <h2 class="text-lg font-bold tracking-tight">Progress</h2>
        <ProgressBar
          class="flex-1"
          :value="progress.sent_count"
          :total="sendable"
          label="Emails sent"
        />
        <span class="text-sm font-bold whitespace-nowrap tabular-nums"
          >{{ formatNumber(progress.sent_count) }} / {{ formatNumber(sendable) }}</span
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

        <p v-if="emails.isPending.value" class="text-muted py-10 text-center text-sm" role="status">
          Loading emails…
        </p>
        <div
          v-else-if="emails.isError.value"
          class="bg-attn text-attn-ink rounded-inner flex flex-wrap items-center justify-between gap-3 p-4"
          role="alert"
        >
          <span class="text-sm"
            ><b>Could not load emails.</b> {{ emails.error.value?.message }}</span
          >
          <button
            type="button"
            class="bg-ink cursor-pointer rounded-full px-4 py-2 text-sm font-semibold text-white"
            @click="() => emails.refetch()"
          >
            Try again
          </button>
        </div>
        <template v-else>
          <JobsTable
            v-if="emailRows.length"
            :jobs="emailRows"
            :retrying-id="retryingJobId"
            @select="openJob"
            @retry="retryOne"
          />
          <p v-else class="text-muted py-10 text-center text-sm">
            {{
              progress.status === 'generating_jobs'
                ? 'Preparing the emails…'
                : 'No emails match these filters.'
            }}
          </p>
          <div v-if="emails.hasNextPage.value" class="mt-3 flex justify-center">
            <AppButton
              variant="secondary"
              :loading="emails.isFetchingNextPage.value"
              @click="emails.fetchNextPage()"
            >
              Load more
            </AppButton>
          </div>
        </template>
      </section>
    </template>

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
      :count="progress.failed_count"
      :busy="retryBatch.isPending.value"
      @confirm="confirmRetryBatch"
    />
    <CancelConfirmDialog
      v-model:visible="cancelOpen"
      :unsent="progress.pending_count"
      :busy="cancelBatch.isPending.value"
      @confirm="confirmCancel"
    />
  </template>
</template>
