<script setup lang="ts">
import { computed } from 'vue'

import SecretField from '@/components/settings/SecretField.vue'
import { describeApply } from '@/lib/settingsGroups'
import type { SettingItem } from '@/types/api'

const props = defineProps<{
  item: SettingItem
  /** The text in the box: the server's value, or the user's unsaved edit. */
  value: string
  error?: string
  secretMode?: 'keep' | 'change' | 'remove'
  secretValue?: string
}>()
const emit = defineEmits<{
  'update:value': [value: string]
  reset: []
  'secret-change': []
  'secret-type': [value: string]
  'secret-remove': []
  'secret-keep': []
}>()

const inputId = computed(() => `setting-${props.item.key}`)
const errorId = computed(() => `${inputId.value}-error`)
const isOn = computed(() => props.value === 'true')
const canReset = computed(() => props.item.editable && props.item.source === 'override')

const box =
  'border-track focus-visible:outline-ink w-full max-w-md rounded-xl border-[1.5px] bg-white px-3.5 py-2.5 text-[0.9rem] font-medium focus-visible:outline-2 focus-visible:outline-offset-2'
</script>

<template>
  <div
    class="border-hairline grid grid-cols-1 gap-2 border-t py-3.5 first:border-t-0 nav:grid-cols-[minmax(9rem,14rem)_1fr] nav:gap-x-5"
  >
    <div>
      <label
        :for="inputId"
        class="mb-0.5 block font-mono text-[0.8125rem] font-semibold break-all"
        >{{ item.key }}</label
      >
      <p class="text-muted text-[0.8125rem] leading-snug">{{ item.description }}</p>
      <div class="mt-2 flex flex-wrap gap-1.5 text-[0.6875rem] font-semibold">
        <span
          class="rounded-full px-2.5 py-0.5"
          :class="item.apply === 'restart' ? 'bg-attn text-attn-ink' : 'bg-pill'"
          >{{ describeApply(item) }}</span
        >
        <span
          class="rounded-full px-2.5 py-0.5"
          :class="item.source === 'override' ? 'bg-ok/10 text-ok-ink' : 'bg-pill'"
          >{{ item.source === 'override' ? 'Changed here' : 'From environment' }}</span
        >
      </div>
      <button
        v-if="canReset"
        type="button"
        class="mt-1.5 text-xs font-semibold underline"
        @click="emit('reset')"
      >
        Reset to default
      </button>
    </div>

    <div>
      <input
        v-if="!item.editable"
        :id="inputId"
        :value="value"
        readonly
        class="bg-note text-muted"
        :class="box"
      />

      <SecretField
        v-else-if="item.secret"
        :id="inputId"
        :has-value="item.has_value"
        :mode="secretMode ?? 'keep'"
        :value="secretValue ?? ''"
        @change="emit('secret-change')"
        @type="emit('secret-type', $event)"
        @remove="emit('secret-remove')"
        @keep="emit('secret-keep')"
      />

      <div v-else-if="item.kind === 'bool'" class="flex items-center gap-3">
        <button
          :id="inputId"
          type="button"
          role="switch"
          :aria-checked="isOn"
          class="relative h-6.5 w-11.5 cursor-pointer rounded-full transition-colors"
          :class="isOn ? 'bg-ink' : 'bg-track'"
          @click="emit('update:value', isOn ? 'false' : 'true')"
        >
          <span
            class="absolute top-0.75 size-5 rounded-full bg-white transition-all"
            :class="isOn ? 'left-6' : 'left-0.75'"
          />
        </button>
        <span class="text-sm" aria-hidden="true">{{ isOn ? 'On' : 'Off' }}</span>
      </div>

      <select
        v-else-if="item.kind === 'enum'"
        :id="inputId"
        :value="value"
        :class="box"
        :aria-describedby="error ? errorId : undefined"
        @change="emit('update:value', ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="option in item.options" :key="option" :value="option">{{ option }}</option>
      </select>

      <input
        v-else
        :id="inputId"
        :value="value"
        :inputmode="item.kind === 'int' ? 'numeric' : undefined"
        :class="[box, error && 'border-bad']"
        :aria-invalid="!!error"
        :aria-describedby="error ? errorId : undefined"
        @input="emit('update:value', ($event.target as HTMLInputElement).value)"
      />

      <p v-if="error" :id="errorId" class="text-bad-ink mt-1.5 text-[0.8125rem]" role="alert">
        {{ error }}
      </p>
    </div>
  </div>
</template>
