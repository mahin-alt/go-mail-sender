<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import PageHeader from '@/components/layout/PageHeader.vue'
import AppButton from '@/components/ui/AppButton.vue'
import FileUpload from '@/components/wizard/FileUpload.vue'
import ReviewReport from '@/components/wizard/ReviewReport.vue'
import { useSpreadsheetUpload } from '@/composables/useSpreadsheetUpload'

const router = useRouter()
const check = useSpreadsheetUpload('verification')

const result = computed(() => check.result.value)
const canContinue = computed(
  () => !!result.value && result.value.valid_count + result.value.unverified_count > 0,
)

/** Send this same checked file: New Bulk Email opens the batch, nothing is uploaded again. */
function continueToBulkEmail() {
  if (!result.value) return
  void router.push({ name: 'new-bulk-email', query: { batch: String(result.value.id) } })
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
        :busy="check.busy.value"
        :busy-text="check.busyText.value"
        :error="check.error.value"
        :max-mb="check.maxUploadMb.value"
        @choose="check.chooseFile"
        @clear="check.clearFile"
      />
    </section>

    <section v-if="result" class="rounded-tile bg-tile p-6.5" aria-labelledby="verify-result">
      <h2 id="verify-result" class="mb-4.5 text-lg font-bold tracking-tight">Result</h2>
      <ReviewReport :result="result" />

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
