<script setup lang="ts" generic="T extends string">
export interface TabOption<V extends string> {
  value: V
  label: string
  /** Shown after the label once known. */
  count?: number
}

defineProps<{ tabs: TabOption<T>[]; label: string }>()
const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="flex flex-wrap gap-1.5" role="group" :aria-label="label">
    <button
      v-for="tab in tabs"
      :key="tab.value"
      type="button"
      class="cursor-pointer rounded-full px-3.5 py-2 text-[0.8125rem] font-semibold"
      :class="model === tab.value ? 'bg-ink text-white' : 'bg-pill text-ink hover:bg-track'"
      :aria-pressed="model === tab.value"
      @click="model = tab.value"
    >
      {{ tab.label }}
      <span
        v-if="tab.count !== undefined"
        class="ml-1.5 font-medium"
        :class="model === tab.value ? 'text-white/80' : 'text-muted'"
        >{{ tab.count.toLocaleString('en') }}</span
      >
    </button>
  </div>
</template>
