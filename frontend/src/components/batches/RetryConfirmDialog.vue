<script setup lang="ts">
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'

defineProps<{ count: number; busy?: boolean }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ confirm: [] }>()

const pt = {
  root: { class: 'rounded-tile bg-tile border-0 w-[min(26rem,calc(100vw-1.75rem))]' },
  header: { class: 'px-6 pt-6 pb-2' },
  title: { class: 'text-lg font-bold tracking-tight' },
  content: { class: 'px-6 pb-2' },
  footer: { class: 'px-6 pb-6' },
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    :closable="false"
    :header="`Retry ${count} failed ${count === 1 ? 'email' : 'emails'}?`"
    :pt="pt"
  >
    <p class="text-muted text-sm">
      Each one will be sent again to its recipient. Invalid rows are skipped.
    </p>
    <template #footer>
      <Button
        rounded
        severity="secondary"
        label="Cancel"
        :disabled="busy"
        @click="visible = false"
      />
      <Button
        rounded
        :label="`Retry ${count} ${count === 1 ? 'email' : 'emails'}`"
        :loading="busy"
        @click="emit('confirm')"
      />
    </template>
  </Dialog>
</template>
