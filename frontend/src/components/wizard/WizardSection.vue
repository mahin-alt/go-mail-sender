<script setup lang="ts">
defineProps<{ number: number; title: string; subtitle: string; locked?: boolean }>()
</script>

<template>
  <!-- `inert` takes a locked section out of the tab order and the screen reader's reach. It must be absent when
       unlocked: the attribute counts as on whenever it exists, even as inert="false". -->
  <section
    class="rounded-tile bg-tile p-6.5 transition-opacity"
    :class="locked && 'opacity-50'"
    :inert="locked || undefined"
    :aria-labelledby="`section-${number}`"
  >
    <h2
      :id="`section-${number}`"
      class="mb-1 flex items-center gap-2.5 text-lg font-bold tracking-tight"
    >
      <span
        class="bg-ink grid size-7 shrink-0 place-items-center rounded-full text-[0.8125rem] text-white"
        aria-hidden="true"
        >{{ number }}</span
      >
      {{ title }}
    </h2>
    <p class="text-muted mb-4.5 text-sm">{{ subtitle }}</p>
    <slot />
  </section>
</template>
