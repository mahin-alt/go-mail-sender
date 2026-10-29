<script setup lang="ts">
import { RotateCw } from '@lucide/vue'

import StatusBadge from '@/components/status/StatusBadge.vue'
import { exactTime, timeAgo } from '@/lib/format'
import type { Job } from '@/types/api'

defineProps<{ jobs: Job[]; retryingId?: number | null }>()
const emit = defineEmits<{ select: [job: Job]; retry: [job: Job] }>()

function onRow(job: Job, event: MouseEvent) {
  if ((event.target as HTMLElement).closest('button')) return
  emit('select', job)
}
</script>

<template>
  <div class="overflow-x-auto">
    <table class="w-full border-collapse text-left text-sm">
      <thead>
        <tr class="text-muted text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
          <th scope="col" class="pr-3 pb-3 pl-2.5 font-semibold">Recipient</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Status</th>
          <th scope="col" class="px-2.5 pb-3 text-right font-semibold">Attempts</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Last error</th>
          <th scope="col" class="px-2.5 pb-3 font-semibold">Updated</th>
          <th scope="col" class="px-2.5 pb-3"><span class="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="job in jobs"
          :key="job.id"
          class="border-hairline hover:bg-note/60 cursor-pointer border-t"
          @click="onRow(job, $event)"
        >
          <td class="py-3 pr-3 pl-2.5">
            <button
              type="button"
              class="text-user cursor-pointer text-left font-semibold hover:underline"
              @click="emit('select', job)"
            >
              {{ job.recipient_name || job.recipient_email }}
            </button>
            <span v-if="job.recipient_name" class="text-muted block text-xs">{{
              job.recipient_email
            }}</span>
          </td>
          <td class="px-2.5 py-3">
            <StatusBadge
              :status="job.status"
              :attempts="job.attempts"
              :max-attempts="job.max_attempts"
            />
          </td>
          <td class="px-2.5 py-3 text-right tabular-nums">{{ job.attempts }}</td>
          <td class="text-bad-ink text-user max-w-64 px-2.5 py-3 text-[0.8125rem]">
            <!-- A sent email keeps the error of its earlier failed attempt; it is history, not a problem. -->
            <span v-if="job.status !== 'sent'" class="line-clamp-2">{{
              job.last_error || ''
            }}</span>
          </td>
          <td class="text-muted px-2.5 py-3 text-[0.8125rem]" :title="exactTime(job.updated_at)">
            {{ timeAgo(job.updated_at) }}
          </td>
          <td class="px-2.5 py-3 text-right">
            <button
              v-if="job.status === 'failed'"
              type="button"
              class="bg-pill hover:bg-track grid size-8 cursor-pointer place-items-center rounded-full disabled:opacity-50"
              :aria-label="`Retry email to ${job.recipient_email}`"
              :disabled="retryingId === job.id"
              @click="emit('retry', job)"
            >
              <RotateCw class="size-4" aria-hidden="true" />
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
