<script setup lang="ts">
import { shallowRef } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import { ApiError } from '@/api/errors'
import type { TestEmailRequest } from '@/api/settings'
import { useSendTestEmail } from '@/queries/useSettings'

/** `connection` holds the SMTP values currently on screen, saved or not. */
const props = defineProps<{ connection: Omit<TestEmailRequest, 'to'>; defaultTo?: string }>()

const to = shallowRef(props.defaultTo ?? '')
const send = useSendTestEmail()
const result = shallowRef<{ ok: boolean; text: string } | null>(null)

function run() {
  result.value = null
  send.mutate(
    { ...props.connection, to: to.value.trim() },
    {
      onSuccess: (message) => (result.value = { ok: true, text: `${message}. Check the inbox.` }),
      onError: (error) =>
        (result.value = {
          ok: false,
          text:
            error instanceof ApiError ? error.message : 'Something went wrong. No email was sent.',
        }),
    },
  )
}
</script>

<template>
  <div class="bg-note rounded-inner mt-3 p-4">
    <h3 class="text-sm font-bold">Send a test email</h3>
    <p class="text-muted mb-2.5 text-[0.8125rem]">
      Uses the values above, even if they are not saved yet.
    </p>
    <div class="flex flex-wrap items-center gap-2.5">
      <input
        v-model="to"
        type="email"
        aria-label="Send the test to"
        placeholder="you@example.com"
        class="border-track focus-visible:outline-ink w-full max-w-72 rounded-xl border-[1.5px] bg-white px-3.5 py-2.5 text-[0.9rem] focus-visible:outline-2 focus-visible:outline-offset-2"
      />
      <AppButton
        variant="secondary"
        :loading="send.isPending.value"
        :disabled="to.trim() === ''"
        @click="run"
      >
        {{ send.isPending.value ? 'Sending…' : 'Send test email' }}
      </AppButton>
    </div>
    <p
      v-if="result"
      class="mt-2.5 text-sm"
      :class="result.ok ? 'text-ok-ink' : 'text-bad-ink'"
      :role="result.ok ? 'status' : 'alert'"
    >
      <b aria-hidden="true">{{ result.ok ? '✓' : '✕' }}</b> {{ result.text }}
    </p>
  </div>
</template>
