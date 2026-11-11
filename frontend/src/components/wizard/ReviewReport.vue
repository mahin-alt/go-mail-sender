<script setup lang="ts">
import { computed, shallowRef } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import KpiTile from '@/components/ui/KpiTile.vue'
import StatusTabs, { type TabOption } from '@/components/ui/StatusTabs.vue'
import { reasonText, TAB_FILTER, tabCount, type ProblemTab } from '@/lib/problems'
import { useIssues } from '@/queries/useBatches'
import type { Batch } from '@/types/api'

const props = defineProps<{ result: Batch }>()

const tab = shallowRef<ProblemTab>('all')

const valid = computed(() => props.result.valid_count + props.result.unverified_count)
const skipped = computed(() => props.result.invalid_count)
const unverified = computed(() => props.result.unverified_count)

const tabs = computed<TabOption<ProblemTab>[]>(() =>
  (
    [
      ['all', 'All problems'],
      ['invalid', 'Invalid'],
      ['domain', 'Bad domain'],
      ['unverified', 'Unverified'],
    ] as const
  ).map(([value, label]) => ({ value, label, count: tabCount(value, props.result) })),
)

// Rows come from the server a page at a time; only the pages looked at are kept.
const problems = useIssues(
  () => props.result.id,
  () => TAB_FILTER[tab.value],
)
const rows = computed(() => problems.data.value?.pages.flatMap((p) => p.items) ?? [])
const shown = computed(() => rows.value.length)
const total = computed(() => tabCount(tab.value, props.result))
const rowTone = computed(() => (tab.value === 'unverified' ? 'text-warn-ink' : 'text-bad-ink'))
</script>

<template>
  <div>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <KpiTile tone="ink" label="Ready to send" :value="valid" caption="valid emails" />
      <KpiTile
        soft
        tone="danger"
        label="Will be skipped"
        :value="skipped"
        caption="invalid or bad domain"
      />
      <KpiTile
        soft
        label="Could not verify"
        :value="unverified"
        caption="still sent (domain check timed out)"
      />
    </div>

    <p
      v-if="valid === 0"
      class="bg-bad/10 text-bad-ink rounded-inner my-3.5 px-4 py-3.5 text-sm"
      role="alert"
    >
      <b>No valid emails in this file.</b>
    </p>
    <p
      v-else-if="skipped === 0"
      class="bg-ok/10 text-ok-ink rounded-inner my-3.5 px-4 py-3.5 text-sm"
    >
      <b>Every row is valid.</b>
    </p>
    <div v-else class="h-3.5" />

    <template v-if="skipped > 0 || unverified > 0">
      <StatusTabs v-model="tab" class="mb-2.5" label="Show problems" :tabs="tabs" />
      <div class="max-h-85 overflow-auto">
        <table class="w-full border-collapse text-left text-sm">
          <thead>
            <tr class="text-muted text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
              <th scope="col" class="bg-tile sticky top-0 py-2 pr-3 pl-2.5 font-semibold">Row</th>
              <th scope="col" class="bg-tile sticky top-0 px-2.5 py-2 font-semibold">Name</th>
              <th scope="col" class="bg-tile sticky top-0 px-2.5 py-2 font-semibold">Email</th>
              <th scope="col" class="bg-tile sticky top-0 px-2.5 py-2 font-semibold">Reason</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in rows" :key="`${tab}-${row.id}`" class="border-hairline border-t">
              <td class="text-muted py-2.5 pr-3 pl-2.5 tabular-nums">{{ row.row }}</td>
              <td class="text-user px-2.5 py-2.5 font-semibold">{{ row.name || '(empty)' }}</td>
              <td class="px-2.5 py-2.5 break-all">{{ row.email || '(empty)' }}</td>
              <td class="text-user px-2.5 py-2.5 text-[0.8125rem]" :class="rowTone">
                {{ reasonText(row) }}
              </td>
            </tr>
            <tr v-if="rows.length === 0 && !problems.isPending.value && !problems.isError.value">
              <td colspan="4" class="text-muted py-6 text-center text-sm">
                Nothing in this group.
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <p v-if="problems.isError.value" class="text-bad-ink mt-2.5 text-sm" role="alert">
        {{ problems.error.value?.message || 'Could not load the rows.' }}
      </p>
      <div
        v-if="shown > 0 && (problems.hasNextPage.value || shown < total)"
        class="mt-2.5 flex items-center gap-3"
      >
        <span class="text-muted text-sm tabular-nums">
          {{ shown.toLocaleString('en') }} of {{ total.toLocaleString('en') }}
        </span>
        <AppButton
          v-if="problems.hasNextPage.value"
          variant="secondary"
          :loading="problems.isFetchingNextPage.value"
          @click="problems.fetchNextPage()"
        >
          Load more
        </AppButton>
      </div>
    </template>
  </div>
</template>
