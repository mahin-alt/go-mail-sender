<script setup lang="ts">
import { computed, shallowRef } from 'vue'

import KpiTile from '@/components/ui/KpiTile.vue'
import StatusTabs, { type TabOption } from '@/components/ui/StatusTabs.vue'
import { problemKind, type ProblemKind } from '@/lib/problems'
import type { ParseResult, RowProblem } from '@/types/api'

const props = defineProps<{ result: ParseResult }>()

type Tab = 'all' | ProblemKind | 'unverified'
const tab = shallowRef<Tab>('all')

const problems = computed(() => props.result.problems)
const unverified = computed(() => props.result.unverified ?? [])
const valid = computed(() => props.result.valid.length)
const skipped = computed(() => problems.value.length)

function count(kind: ProblemKind) {
  return problems.value.filter((p) => problemKind(p) === kind).length
}

const tabs = computed<TabOption<Tab>[]>(() => [
  { value: 'all', label: 'All problems', count: skipped.value },
  { value: 'invalid', label: 'Invalid', count: count('invalid') },
  { value: 'duplicate', label: 'Duplicate', count: count('duplicate') },
  { value: 'domain', label: 'Bad domain', count: count('domain') },
  { value: 'unverified', label: 'Unverified', count: unverified.value.length },
])

const rows = computed<RowProblem[]>(() => {
  if (tab.value === 'unverified') return unverified.value
  if (tab.value === 'all') return problems.value
  return problems.value.filter((p) => problemKind(p) === tab.value)
})
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
        caption="invalid, duplicate or bad domain"
      />
      <KpiTile
        soft
        label="Could not verify"
        :value="result.unverified_count"
        caption="still sent (domain check timed out)"
      />
    </div>

    <p
      v-if="valid === 0"
      class="bg-bad/10 text-bad-ink rounded-inner my-3.5 px-4 py-3.5 text-sm"
      role="alert"
    >
      <b>No valid emails in this file.</b> Fix the problems below and upload it again.
    </p>
    <p
      v-else-if="skipped > 0"
      class="bg-attn text-attn-ink rounded-inner my-3.5 px-4 py-3.5 text-sm leading-relaxed"
    >
      <b>{{ skipped }} {{ skipped === 1 ? 'row' : 'rows' }} will be skipped.</b>
      You can send to the {{ valid }} valid {{ valid === 1 ? 'email' : 'emails' }} now, or remove
      the file and upload a corrected one.
    </p>
    <p v-else class="bg-ok/10 text-ok-ink rounded-inner my-3.5 px-4 py-3.5 text-sm">
      <b>Every row is valid.</b>
    </p>

    <template v-if="skipped > 0 || unverified.length > 0">
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
            <tr v-for="row in rows" :key="`${tab}-${row.row}`" class="border-hairline border-t">
              <td class="text-muted py-2.5 pr-3 pl-2.5 tabular-nums">{{ row.row }}</td>
              <td class="text-user px-2.5 py-2.5 font-semibold">{{ row.name || '(empty)' }}</td>
              <td class="px-2.5 py-2.5 break-all">{{ row.email || '(empty)' }}</td>
              <td class="text-user px-2.5 py-2.5 text-[0.8125rem]" :class="rowTone">
                {{ row.reason }}
              </td>
            </tr>
            <tr v-if="rows.length === 0">
              <td colspan="4" class="text-muted py-6 text-center text-sm">
                Nothing in this group.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
