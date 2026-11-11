<script setup lang="ts">
import { useRouter } from 'vue-router'

import ProgressBar from '@/components/ui/ProgressBar.vue'
import StatusBadge from '@/components/status/StatusBadge.vue'
import { exactTime, formatNumber, timeAgo } from '@/lib/format'
import type { Batch } from '@/types/api'

defineProps<{ items: Batch[] }>()

/** Rows that get (or got) an email: valid + unverified. */
const sendable = (b: Batch) => b.valid_count + b.unverified_count

const router = useRouter()

/** Clicking anywhere on the row opens the batch; the filename link is the keyboard way in. */
function open(id: number, event: MouseEvent) {
  if ((event.target as HTMLElement).closest('a,button')) return
  void router.push({ name: 'bulk-email-detail', params: { id } })
}
</script>

<template>
  <div class="overflow-x-auto">
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr class="text-muted text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
          <th scope="col" class="pr-3 pb-3 pl-2.5 font-semibold">Filename</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Status</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Progress</th>
          <th scope="col" class="px-2.5 pb-3 text-right font-semibold">Failed</th>
          <th scope="col" class="px-2.5 pb-3 text-right font-semibold">Invalid</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Created</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="batch in items"
          :key="batch.id"
          class="border-hairline hover:bg-note/60 cursor-pointer border-t"
          @click="open(batch.id, $event)"
        >
          <td class="py-3.5 pr-3 pl-2.5">
            <RouterLink
              :to="{ name: 'bulk-email-detail', params: { id: batch.id } }"
              class="text-user font-semibold hover:underline"
              >{{ batch.filename || `Batch #${batch.id}` }}</RouterLink
            >
            <span class="text-muted block text-xs">#{{ batch.id }}</span>
          </td>
          <td class="px-2.5 py-3.5">
            <StatusBadge :status="batch.status" />
          </td>
          <td class="px-2.5 py-3.5">
            <ProgressBar
              :value="batch.sent_count"
              :total="sendable(batch)"
              :label="`${batch.filename} progress`"
            />
            <span class="text-muted mt-1 block text-xs"
              >{{ formatNumber(batch.sent_count) }} / {{ formatNumber(sendable(batch)) }}</span
            >
          </td>
          <td
            class="px-2.5 py-3.5 text-right tabular-nums"
            :class="batch.failed_count > 0 && 'text-bad-ink font-semibold'"
          >
            {{ formatNumber(batch.failed_count) }}
          </td>
          <td class="px-2.5 py-3.5 text-right tabular-nums">{{ formatNumber(batch.invalid_count) }}</td>
          <td
            class="text-muted px-2.5 py-3.5 text-[0.8125rem]"
            :title="exactTime(batch.created_at)"
          >
            {{ timeAgo(batch.created_at) }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
