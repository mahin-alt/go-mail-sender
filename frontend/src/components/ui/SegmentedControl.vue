<script setup lang="ts" generic="T extends string | number">
export interface SegmentOption<V extends string | number> {
  value: V
  label: string
}

defineProps<{ options: SegmentOption<T>[]; label: string }>()
const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="bg-pill flex gap-1 rounded-full p-0.75" role="group" :aria-label="label">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="cursor-pointer rounded-full px-3 py-1.5 text-[0.78rem] font-semibold"
      :class="model === option.value ? 'bg-ink text-white' : 'text-ink hover:bg-track'"
      :aria-pressed="model === option.value"
      @click="model = option.value"
    >
      {{ option.label }}
    </button>
  </div>
</template>
