<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'

const props = defineProps<{
  title: string
  message: string
  confirmLabel: string
  busy?: boolean
  /** Centre the title and the message, with more room before the buttons. */
  centered?: boolean
  /** Label of the button that closes without acting (default "Cancel"). */
  dismissLabel?: string
  /** Show the confirm button in the danger colour, for actions that cannot be undone. */
  danger?: boolean
}>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ confirm: [] }>()

const pt = computed(() => ({
  root: { class: 'rounded-tile bg-tile border-0 w-[min(26rem,calc(100vw-1.75rem))]' },
  header: { class: props.centered ? 'justify-center px-6 pt-7 pb-2' : 'px-6 pt-6 pb-2' },
  title: {
    class: props.centered
      ? 'text-lg font-bold tracking-tight text-center'
      : 'text-lg font-bold tracking-tight',
  },
  content: { class: props.centered ? 'px-6 pb-7' : 'px-6 pb-2' },
  footer: { class: 'px-6 pb-6' },
}))
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    :closable="false"
    :draggable="false"
    :header="title"
    :pt="pt"
  >
    <p class="text-muted text-sm" :class="centered && 'text-center'">{{ message }}</p>
    <template #footer>
      <Button
        rounded
        severity="secondary"
        :label="dismissLabel ?? 'Cancel'"
        :disabled="busy"
        @click="visible = false"
      />
      <Button
        rounded
        :severity="danger ? 'danger' : undefined"
        :label="confirmLabel"
        :loading="busy"
        @click="emit('confirm')"
      />
    </template>
  </Dialog>
</template>
