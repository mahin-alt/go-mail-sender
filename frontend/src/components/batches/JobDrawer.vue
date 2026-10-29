<script setup lang="ts">
import { RotateCw } from '@lucide/vue'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'

import StatusBadge from '@/components/status/StatusBadge.vue'
import { exactTime } from '@/lib/format'
import type { Job } from '@/types/api'

defineProps<{ job: Job | null; retrying?: boolean }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ retry: [job: Job] }>()

/** Floating panel: inset from the screen edge with the tile radius, like every tile. */
const pt = {
  root: {
    class:
      'rounded-tile bg-tile !m-3.5 !h-[calc(100%-1.75rem)] w-[min(27.5rem,calc(100vw-1.75rem))] border-0 shadow-xl',
  },
  header: { class: 'px-6 pt-5.5 pb-3.5' },
  title: { class: 'text-lg font-bold tracking-tight' },
  content: { class: 'px-6 pb-3' },
  footer: { class: 'px-6 pb-5.5' },
}
</script>

<template>
  <Drawer
    v-model:visible="visible"
    position="right"
    :header="job?.recipient_name || job?.recipient_email"
    :pt="pt"
  >
    <div v-if="job" class="flex flex-col gap-4">
      <p class="text-muted -mt-1 text-[0.8125rem]">
        Email #{{ job.id }} · updated {{ exactTime(job.updated_at) }}
      </p>
      <div>
        <StatusBadge
          :status="job.status"
          :attempts="job.attempts"
          :max-attempts="job.max_attempts"
        />
      </div>

      <div
        v-if="job.last_error && job.status !== 'sent'"
        class="bg-attn text-attn-ink rounded-inner text-user px-3.5 py-3 text-[0.84rem]"
      >
        <b class="block">Last error</b>
        {{ job.last_error }}
      </div>

      <dl class="grid grid-cols-2 gap-3.5 text-sm">
        <div>
          <dt class="text-muted mb-1 text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
            Recipient
          </dt>
          <dd class="break-all">{{ job.recipient_email }}</dd>
        </div>
        <div>
          <dt class="text-muted mb-1 text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
            Attempts
          </dt>
          <dd>{{ job.attempts }} of {{ job.max_attempts }}</dd>
        </div>
      </dl>

      <div>
        <h3 class="text-muted mb-1 text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
          Subject
        </h3>
        <p class="text-user text-sm">{{ job.subject }}</p>
      </div>
      <div>
        <h3 class="text-muted mb-1 text-[0.6875rem] font-semibold tracking-[0.06em] uppercase">
          Message
        </h3>
        <!-- Plain text on purpose: the body comes from an uploaded spreadsheet, never render it as HTML. -->
        <pre
          class="bg-note rounded-inner text-user font-sans text-[0.84rem] break-words whitespace-pre-wrap p-3.5"
          >{{ job.body }}</pre>
      </div>
    </div>

    <template #footer>
      <div v-if="job" class="flex justify-end gap-2.5">
        <Button
          v-if="job.status === 'failed'"
          rounded
          :disabled="retrying"
          aria-label="Retry this email"
          @click="emit('retry', job)"
        >
          <RotateCw class="size-4" aria-hidden="true" /> Retry
        </Button>
        <Button rounded severity="secondary" label="Close" @click="visible = false" />
      </div>
    </template>
  </Drawer>
</template>
