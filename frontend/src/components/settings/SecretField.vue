<script setup lang="ts">
import AppButton from '@/components/ui/AppButton.vue'

defineProps<{
  id: string
  hasValue: boolean
  mode: 'keep' | 'change' | 'remove'
  value: string
}>()
const emit = defineEmits<{ change: []; type: [value: string]; remove: []; keep: [] }>()
</script>

<template>
  <div class="flex flex-wrap items-center gap-3 text-sm">
    <template v-if="mode === 'change'">
      <!-- The saved password is never sent to the browser, so this box only ever holds a NEW one. -->
      <input
        :id="id"
        type="password"
        autocomplete="new-password"
        placeholder="New password"
        :value="value"
        class="border-track focus-visible:outline-ink w-full max-w-sm rounded-xl border-[1.5px] bg-white px-3.5 py-2.5 text-[0.9rem] focus-visible:outline-2 focus-visible:outline-offset-2"
        @input="emit('type', ($event.target as HTMLInputElement).value)"
      />
      <button type="button" class="font-semibold underline" @click="emit('keep')">Cancel</button>
    </template>
    <template v-else-if="mode === 'remove'">
      <span>The password will be removed when you save.</span>
      <button type="button" class="font-semibold underline" @click="emit('keep')">Undo</button>
    </template>
    <template v-else>
      <span
        :id="id"
        class="rounded-full px-3 py-1 text-xs font-semibold"
        :class="hasValue ? 'bg-ok/10 text-ok-ink' : 'bg-pill text-muted'"
        >{{ hasValue ? 'Password is set' : 'No password' }}</span
      >
      <AppButton variant="secondary" @click="emit('change')">{{
        hasValue ? 'Change password' : 'Set password'
      }}</AppButton>
      <button v-if="hasValue" type="button" class="font-semibold underline" @click="emit('remove')">
        Remove
      </button>
    </template>
  </div>
</template>
