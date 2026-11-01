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
import SendBar from '@/components/wizard/SendBar.vue'
import WizardSection from '@/components/wizard/WizardSection.vue'
import { useBulkEmailDraft } from '@/composables/useBulkEmailDraft'
import { formatNumber } from '@/lib/format'
import { summarizeCheck } from '@/lib/problems'
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
const summary = computed(() => (draft.result.value ? summarizeCheck(draft.result.value) : ''))

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
  <PageHeader
    title="New Bulk Email"
    subtitle="Upload, review, write, send. Nothing is sent until you confirm."
  />

  <div class="flex flex-col gap-3.5">
    <WizardSection
      :number="1"
      title="Upload your spreadsheet"
      subtitle="We check the file first. Nothing is sent or saved until you confirm at the end."
    >
      <FileUpload
        :file="draft.file.value"
        :checking="draft.checking.value"
        :error="uploadError"
        :summary="summary"
        @choose="draft.chooseFile"
        @clear="draft.clearFile"
      />
    </WizardSection>

    <WizardSection
      :number="2"
      title="Review the file"
      subtitle="What will happen when you send. Fix the file and upload it again if something looks wrong."
      :locked="!draft.ready.value"
    >
      <ReviewReport v-if="draft.result.value" :result="draft.result.value" />
      <p v-else class="text-muted text-sm">Upload a file first to see how many rows are valid.</p>
    </WizardSection>

    <WizardSection
      :number="3"
      title="Write your message"
      subtitle="Prefilled with a starting message. Edit it, then check the preview."
      :locked="!draft.ready.value"
    >
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

  <SendBar
    :ready="draft.ready.value"
    :valid="draft.validCount.value"
    :skipped="draft.skippedCount.value"
    :blocker="draft.blocker.value"
    @send="confirmOpen = true"
  />

  <ConfirmDialog
    v-model:visible="confirmOpen"
    :title="`Send to ${formatNumber(draft.validCount.value)} ${draft.validCount.value === 1 ? 'recipient' : 'recipients'}?`"
    :message="`Each valid row will get the email${draft.skippedCount.value ? `, and ${formatNumber(draft.skippedCount.value)} skipped ${draft.skippedCount.value === 1 ? 'row is' : 'rows are'} left out` : ''}. This cannot be undone.`"
    :confirm-label="`Send ${formatNumber(draft.validCount.value)} ${draft.validCount.value === 1 ? 'email' : 'emails'}`"
    :busy="draft.sending.value"
    @confirm="confirmSend"
  />
</template>
