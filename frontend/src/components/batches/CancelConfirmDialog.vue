<script setup lang="ts">
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { formatNumber } from '@/lib/format'

defineProps<{ unsent: number; busy?: boolean }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ confirm: [] }>()
</script>

<template>
  <ConfirmDialog
    v-model:visible="visible"
    title="Stop this Bulk Email?"
    :message="`${formatNumber(unsent)} unsent ${unsent === 1 ? 'email' : 'emails'} will not be sent. This cannot be undone.`"
    confirm-label="Stop sending"
    dismiss-label="Keep sending"
    danger
    :busy="busy"
    @confirm="emit('confirm')"
  />
</template>
