<script setup lang="ts">
import { onBeforeUnmount, shallowRef, watch } from 'vue'

const props = withDefaults(defineProps<{ label: string; placeholder?: string; delay?: number }>(), {
  placeholder: '',
  delay: 350,
})
/** The committed value. It updates `delay` ms after typing stops, so every key press is not a request. */
const model = defineModel<string>({ required: true })

const text = shallowRef(model.value)
let timer: ReturnType<typeof setTimeout> | undefined

watch(model, (value) => {
  if (value !== text.value) text.value = value // changed from outside (Back button, clear)
})

function onInput() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    model.value = text.value.trim()
  }, props.delay)
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <input
    v-model="text"
    type="search"
    :placeholder="placeholder"
    :aria-label="label"
    class="border-track focus-visible:outline-ink min-w-60 rounded-full border-[1.5px] bg-white px-4 py-2 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
    @input="onInput"
  />
</template>
