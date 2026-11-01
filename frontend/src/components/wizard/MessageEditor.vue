<script setup lang="ts">
import { shallowRef, useTemplateRef } from 'vue'

import { TEMPLATE_FIELDS } from '@/lib/template'

defineProps<{ badFields: string[] }>()
const subject = defineModel<string>('subject', { required: true })
const body = defineModel<string>('body', { required: true })

const subjectInput = useTemplateRef<HTMLInputElement>('subjectInput')
const bodyInput = useTemplateRef<HTMLTextAreaElement>('bodyInput')
const lastFocused = shallowRef<'subject' | 'body'>('body')

/** `{{.Field}}`; built in script because a template expression cannot contain the closing braces. */
const token = (field: string) => `{{.${field}}}`
const allowed = TEMPLATE_FIELDS.map(token).join(' and ')

/** Put `{{.Field}}` where the cursor is in whichever box was used last. */
function insert(field: string) {
  const text = token(field)
  const el = lastFocused.value === 'subject' ? subjectInput.value : bodyInput.value
  if (!el) return
  const start = el.selectionStart ?? el.value.length
  const end = el.selectionEnd ?? start
  const next = el.value.slice(0, start) + text + el.value.slice(end)
  if (lastFocused.value === 'subject') subject.value = next
  else body.value = next
  const caret = start + text.length
  requestAnimationFrame(() => {
    el.focus()
    el.setSelectionRange(caret, caret)
  })
}

const box =
  'border-track focus-visible:outline-ink w-full rounded-inner border-[1.5px] bg-white px-4 py-3 text-[0.9375rem] font-medium leading-relaxed focus-visible:outline-2 focus-visible:outline-offset-2'
const label = 'text-muted mb-1.5 block text-[0.6875rem] font-semibold tracking-[0.06em] uppercase'
</script>

<template>
  <div>
    <div class="text-muted mb-2 flex flex-wrap items-center gap-2 text-[0.8125rem]">
      Insert a field:
      <button
        v-for="field in TEMPLATE_FIELDS"
        :key="field"
        type="button"
        class="bg-pill hover:bg-track text-ink cursor-pointer rounded-full px-3 py-1.5 font-mono text-[0.8125rem] font-semibold"
        @click="insert(field)"
      >
        {{ token(field) }}
      </button>
    </div>

    <div class="mb-4">
      <label for="subject" :class="label">Subject</label>
      <input
        id="subject"
        ref="subjectInput"
        v-model="subject"
        type="text"
        :class="[box, 'text-user']"
        @focus="lastFocused = 'subject'"
      />
    </div>
    <div class="mb-4">
      <label for="message" :class="label">Message</label>
      <textarea
        id="message"
        ref="bodyInput"
        v-model="body"
        rows="9"
        :class="[box, 'text-user min-h-52 resize-y']"
        @focus="lastFocused = 'body'"
      />
    </div>

    <p
      v-if="badFields.length"
      class="bg-bad/10 text-bad-ink rounded-inner px-4 py-3 text-sm"
      role="alert"
    >
      Unknown {{ badFields.length === 1 ? 'field' : 'fields' }}
      <b v-for="(f, i) in badFields" :key="f">{{ i ? ', ' : '' }}{{ token(f) }}</b
      >. Only {{ allowed }} work.
    </p>
  </div>
</template>
