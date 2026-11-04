<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import PageHeader from '@/components/layout/PageHeader.vue'
import AppButton from '@/components/ui/AppButton.vue'
import FileUpload from '@/components/wizard/FileUpload.vue'
import ReviewReport from '@/components/wizard/ReviewReport.vue'
import { useRecipientCheck } from '@/composables/useRecipientCheck'
import { usePendingCheckStore } from '@/stores/pendingCheck'

const router = useRouter()
const pending = usePendingCheckStore()
const check = useRecipientCheck()

const uploadError = computed(() => check.fileError.value || check.parseError.value)
const canContinue = computed(() => (check.result.value?.valid.length ?? 0) > 0)

/** Carry the checked file over so New Bulk Email starts from it instead of asking for it again. */
function continueToBulkEmail() {
  const { file, result } = check
  if (!file.value || !result.value) return
  pending.hold(file.value, result.value)
  void router.push({ name: 'new-bulk-email' })
}
</script>

<template>
  <PageHeader title="Verify Emails" />

  <div class="flex flex-col gap-3.5">
    <section class="rounded-tile bg-tile p-6.5" aria-labelledby="verify-upload">
      <h2 id="verify-upload" class="mb-4.5 text-lg font-bold tracking-tight">
        Upload your spreadsheet
      </h2>
      <FileUpload
        :file="check.file.value"
        :checking="check.checking.value"
        :error="uploadError"
        :max-mb="check.maxUploadMb.value"
        @choose="check.chooseFile"
        @clear="check.clearFile"
      />
    </section>

    <section
      v-if="check.result.value"
      class="rounded-tile bg-tile p-6.5"
      aria-labelledby="verify-result"
    >
      <h2 id="verify-result" class="mb-4.5 text-lg font-bold tracking-tight">Result</h2>
      <ReviewReport :result="check.result.value" />

      <div class="border-hairline mt-4 flex flex-wrap items-center gap-2.5 border-t pt-4">
        <AppButton variant="secondary" @click="check.clearFile">Check another file</AppButton>
        <AppButton
          class="ml-auto"
          :disabled="!canContinue"
          :title="canContinue ? '' : 'The file has no valid emails to send'"
          @click="continueToBulkEmail"
        >
          Continue to New Bulk Email
        </AppButton>
      </div>
    </section>
  </div>
</template>
