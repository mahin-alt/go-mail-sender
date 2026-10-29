<script setup lang="ts">
defineProps<{ title: string; subtitle?: string }>()
</script>

<template>
  <header class="mb-5 flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="text-[2.5rem] leading-tight font-bold tracking-tight">{{ title }}</h1>
      <p v-if="subtitle" class="text-muted mt-1 text-sm">{{ subtitle }}</p>
    </div>
    <div class="flex items-center gap-3"><slot name="actions" /></div>
  </header>
</template>
