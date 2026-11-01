<script setup lang="ts">
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

defineProps<{ count: number; busy?: boolean }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ confirm: [] }>()
</script>

<template>
  <ConfirmDialog
    v-model:visible="visible"
    :title="`Retry ${count} failed ${count === 1 ? 'email' : 'emails'}?`"
    message="Each one will be sent again to its recipient. Invalid rows are skipped."
    :confirm-label="`Retry ${count} ${count === 1 ? 'email' : 'emails'}`"
    :busy="busy"
    @confirm="emit('confirm')"
  />
</template>
