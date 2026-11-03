<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'

import { ApiError } from '@/api/errors'
import PageHeader from '@/components/layout/PageHeader.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import EmailPreview from '@/components/wizard/EmailPreview.vue'
import FileUpload from '@/components/wizard/FileUpload.vue'
import MessageEditor from '@/components/wizard/MessageEditor.vue'
import ReviewReport from '@/components/wizard/ReviewReport.vue'
import SendButton from '@/components/wizard/SendButton.vue'
import WizardSection from '@/components/wizard/WizardSection.vue'
import { useBulkEmailDraft } from '@/composables/useBulkEmailDraft'
import { formatNumber } from '@/lib/format'
import { fillTemplate } from '@/lib/template'
import { usePendingCheckStore } from '@/stores/pendingCheck'

const router = useRouter()
const toast = useToast()
const draft = useBulkEmailDraft()

const confirmOpen = shallowRef(false)

// Arriving from Verify Emails with a file that is already checked: start from it.
const pending = usePendingCheckStore().take()
if (pending) draft.adopt(pending.file, pending.result)

const uploadError = computed(() => draft.fileError.value || draft.parseError.value)
const current = computed(() => draft.samples.value[draft.previewIndex.value])
const previewSubject = computed(() =>
  current.value ? fillTemplate(draft.subject.value, current.value) : draft.subject.value,
)
const previewBody = computed(() =>
  current.value ? fillTemplate(draft.body.value, current.value) : draft.body.value,
)
const previewTo = computed(() =>
  current.value ? `${current.value.name} <${current.value.email}>` : '',
)

/** "296 valid recipients will receive emails. 16 invalid entries will be excluded from sending." */
const confirmMessage = computed(() => {
  const sent = draft.validCount.value
  const skipped = draft.skippedCount.value
  const receive = `${formatNumber(sent)} valid ${sent === 1 ? 'recipient' : 'recipients'} will receive emails.`
  if (skipped === 0) return receive
  return `${receive} ${formatNumber(skipped)} invalid ${skipped === 1 ? 'entry' : 'entries'} will be excluded from sending.`
})

async function confirmSend() {
  try {
    const batch = await draft.send()
    confirmOpen.value = false
    toast.add({
      severity: 'success',
      summary: 'Bulk Email queued',
      detail: `${formatNumber(batch.valid_count)} ${batch.valid_count === 1 ? 'email is' : 'emails are'} being sent.`,
      life: 5000,
    })
    await router.push({ name: 'bulk-email-detail', params: { id: batch.batch_id } })
  } catch (error) {
    confirmOpen.value = false
    toast.add({
      severity: 'error',
      summary: 'Could not send',
      detail:
        error instanceof ApiError
          ? error.message
          : 'Something went wrong. Nothing was sent. Please try again.',
    })
  }
}
</script>

<template>
  <PageHeader title="New Bulk Email" />

  <div class="flex flex-col gap-3.5">
    <WizardSection :number="1" title="Upload your spreadsheet">
      <FileUpload
        :file="draft.file.value"
        :checking="draft.checking.value"
        :error="uploadError"
        @choose="draft.chooseFile"
        @clear="draft.clearFile"
      />
    </WizardSection>

    <WizardSection :number="2" title="Review the file" :locked="!draft.ready.value">
      <ReviewReport v-if="draft.result.value" :result="draft.result.value" />
      <p v-else class="text-muted text-sm">Upload a file first.</p>
    </WizardSection>

    <WizardSection :number="3" title="Write your message" :locked="!draft.ready.value">
      <div class="grid grid-cols-1 gap-5 nav:grid-cols-[1.1fr_1fr]">
        <MessageEditor
          v-model:subject="draft.subject.value"
          v-model:body="draft.body.value"
          :bad-fields="draft.badFields.value"
        />
        <EmailPreview
          :to="previewTo || 'recipient'"
          :subject="previewSubject"
          :body="previewBody"
          :position="draft.previewIndex.value + 1"
          :total="Math.max(draft.samples.value.length, 1)"
          @previous="draft.step(-1)"
          @next="draft.step(1)"
        />
      </div>
    </WizardSection>
  </div>

  <SendButton
    :valid="draft.validCount.value"
    :blocker="draft.blocker.value"
    @send="confirmOpen = true"
  />

  <ConfirmDialog
    v-model:visible="confirmOpen"
    centered
    title="Confirm"
    :message="confirmMessage"
    confirm-label="Send"
    :busy="draft.sending.value"
    @confirm="confirmSend"
  />
</template>
