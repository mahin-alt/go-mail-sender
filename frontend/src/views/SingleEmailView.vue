<script setup lang="ts">
import { computed, shallowRef } from 'vue'

import PageHeader from '@/components/layout/PageHeader.vue'
import AppButton from '@/components/ui/AppButton.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { useSingleEmail } from '@/composables/useSingleEmail'
import { useSettings } from '@/queries/useSettings'

const email = useSingleEmail()
const confirmOpen = shallowRef(false)

/** The address emails are sent from, shown in the window's header. Fine to be missing. */
const settings = useSettings()
const from = computed(
  () => settings.data.value?.find((item) => item.key === 'MAIL_FROM')?.value ?? '',
)

function onSend() {
  if (email.validate()) confirmOpen.value = true
}

async function confirmSend() {
  await email.submit()
  confirmOpen.value = false
}

const line = 'border-hairline flex items-center gap-3 border-b px-5.5 py-1.5'
const label = 'text-muted w-16 shrink-0 text-xs font-semibold tracking-[0.06em] uppercase'
const bare =
  'text-user min-w-0 flex-1 bg-transparent py-3 text-[0.9375rem] font-medium outline-none'
const err = 'text-bad-ink px-5.5 py-1.5 text-[0.8125rem]'
</script>

<template>
  <PageHeader title="Single Email" />

  <div class="max-w-3xl">
    <p
      v-if="email.sentTo.value"
      class="bg-ok/10 text-ok-ink rounded-tile mb-3.5 px-5.5 py-4.5"
      role="status"
    >
      <b class="mb-0.5 block text-base">Email sent</b>
      Sent to {{ email.sentTo.value }}.
      <button type="button" class="ml-1.5 font-semibold underline" @click="email.reset">
        Send another
      </button>
    </p>
    <p
      v-if="email.failure.value"
      class="bg-bad/10 text-bad-ink rounded-tile mb-3.5 px-5.5 py-4.5 text-sm"
      role="alert"
    >
      <b>Could not send.</b> {{ email.failure.value }} Nothing was sent.
    </p>

    <form class="rounded-tile bg-tile overflow-hidden" novalidate @submit.prevent="onSend">
      <div
        class="bg-ink flex items-center justify-between px-5.5 py-3 text-sm font-semibold text-white"
      >
        <span>New message</span>
      </div>

      <div v-if="from" :class="line">
        <label for="single-from" :class="label">From</label>
        <input
          id="single-from"
          :value="from"
          type="text"
          readonly
          aria-readonly="true"
          :class="[bare, 'text-muted cursor-default']"
        />
      </div>

      <div :class="line">
        <label for="single-to" :class="label">To</label>
        <input
          id="single-to"
          v-model="email.to.value"
          type="email"
          placeholder="name@example.com"
          autocomplete="off"
          :class="bare"
          :aria-invalid="email.showErrors.value && !!email.errors.value.to"
          :aria-describedby="
            email.showErrors.value && email.errors.value.to ? 'single-to-error' : undefined
          "
          @input="email.edited"
        />
      </div>
      <p
        v-if="email.showErrors.value && email.errors.value.to"
        id="single-to-error"
        :class="err"
        role="alert"
      >
        {{ email.errors.value.to }}
      </p>

      <div :class="line">
        <label for="single-subject" :class="label">Subject</label>
        <input
          id="single-subject"
          v-model="email.subject.value"
          type="text"
          :class="bare"
          :aria-invalid="email.showErrors.value && !!email.errors.value.subject"
          :aria-describedby="
            email.showErrors.value && email.errors.value.subject
              ? 'single-subject-error'
              : undefined
          "
          @input="email.edited"
        />
      </div>
      <p
        v-if="email.showErrors.value && email.errors.value.subject"
        id="single-subject-error"
        :class="err"
        role="alert"
      >
        {{ email.errors.value.subject }}
      </p>

      <label for="single-body" class="sr-only">Message</label>
      <textarea
        id="single-body"
        v-model="email.body.value"
        placeholder="Write your message…"
        class="text-user block min-h-64 w-full resize-y px-5.5 py-4.5 text-[0.9375rem] leading-relaxed font-medium outline-none"
        :aria-invalid="email.showErrors.value && !!email.errors.value.body"
        :aria-describedby="
          email.showErrors.value && email.errors.value.body ? 'single-body-error' : undefined
        "
        @input="email.edited"
      />
      <p
        v-if="email.showErrors.value && email.errors.value.body"
        id="single-body-error"
        :class="err"
        role="alert"
      >
        {{ email.errors.value.body }}
      </p>

      <div
        class="border-hairline flex flex-wrap items-center justify-between gap-2.5 border-t px-5.5 py-3.5"
      >
        <span />
        <AppButton type="submit" :loading="email.sending.value">{{
          email.sending.value ? 'Sending…' : 'Send'
        }}</AppButton>
      </div>
    </form>
  </div>

  <ConfirmDialog
    v-model:visible="confirmOpen"
    title="Send this email now?"
    :message="`It will be sent to ${email.to.value.trim()} immediately. This cannot be undone.`"
    confirm-label="Send email"
    :busy="email.sending.value"
    @confirm="confirmSend"
  />
</template>
