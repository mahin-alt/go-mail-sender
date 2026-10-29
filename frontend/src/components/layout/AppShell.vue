<script setup lang="ts">
import AppSidebar from '@/components/layout/AppSidebar.vue'
import { useSidebar } from '@/composables/useSidebar'

const { pinned, expanded, toggle, onPointerEnter, onPointerLeave, onFocusIn, onFocusOut } =
  useSidebar()
</script>

<template>
  <div
    class="min-h-screen transition-[padding] duration-200 motion-reduce:transition-none"
    :class="expanded ? 'nav:pl-shell-open' : 'nav:pl-shell-rail'"
  >
    <a
      href="#content"
      class="bg-ink focus:ring-ink absolute top-2 left-2 z-30 -translate-y-16 rounded-full px-4 py-2 text-sm font-semibold text-white focus:translate-y-0"
      >Skip to content</a
    >
    <AppSidebar
      :pinned="pinned"
      :expanded="expanded"
      @toggle="toggle"
      @pointerenter="onPointerEnter"
      @pointerleave="onPointerLeave"
      @focusin="onFocusIn"
      @focusout="onFocusOut"
    />
    <main
      id="content"
      class="mx-auto w-full max-w-[80rem] px-4 pt-3 pb-10 nav:pr-8.5 nav:pl-5 nav:pt-7.5"
    >
      <slot />
    </main>
  </div>
</template>
