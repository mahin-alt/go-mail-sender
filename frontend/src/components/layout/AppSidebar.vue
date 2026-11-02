<script setup lang="ts">
import { ChevronsLeft, ChevronsRight } from '@lucide/vue'

import BrandLogo from '@/components/brand/BrandLogo.vue'
import ApiStatus from '@/components/layout/ApiStatus.vue'
import { useNavItems } from '@/composables/useNavItems'

defineProps<{
  /** Pinned open (true) or collapsed to the icon rail (false). */
  pinned: boolean
  /** Whether labels are showing right now (pinned, or opened by hover/keyboard focus). */
  expanded: boolean
}>()

const emit = defineEmits<{
  toggle: []
  pointerenter: []
  pointerleave: []
  focusin: [event: FocusEvent]
  focusout: [event: FocusEvent]
}>()

const items = useNavItems()
</script>

<template>
  <aside
    class="rounded-tile bg-tile nav:fixed nav:inset-y-3.5 nav:left-3.5 nav:m-0 nav:flex-col nav:items-stretch nav:gap-5 nav:overflow-hidden nav:px-2.5 nav:py-4.5 nav:transition-[width] nav:duration-200 motion-reduce:nav:transition-none z-20 m-2 flex items-center gap-2 overflow-x-auto p-2"
    :class="expanded ? 'nav:w-sidebar-open' : 'nav:w-sidebar-rail'"
    aria-label="Sidebar"
    @mouseenter="emit('pointerenter')"
    @mouseleave="emit('pointerleave')"
    @focusin="emit('focusin', $event)"
    @focusout="emit('focusout', $event)"
  >
    <div class="flex items-center justify-between gap-1 nav:gap-2">
      <BrandLogo :show-wordmark="expanded" />
      <!-- Icon only, beside the logo. In the narrow rail it stays hidden until the sidebar opens (hover or keyboard focus). -->
      <button
        type="button"
        class="rounded-inner text-muted hidden size-9 shrink-0 cursor-pointer place-items-center transition-opacity duration-150 hover:bg-hover hover:text-ink motion-reduce:transition-none nav:grid"
        :class="expanded ? 'opacity-100' : 'pointer-events-none opacity-0'"
        :aria-expanded="pinned"
        :aria-label="pinned ? 'Collapse sidebar' : 'Keep sidebar open'"
        :title="pinned ? 'Collapse sidebar' : 'Keep sidebar open'"
        @click="emit('toggle')"
      >
        <component :is="pinned ? ChevronsLeft : ChevronsRight" class="size-5" aria-hidden="true" />
      </button>
    </div>

    <nav class="flex gap-1 nav:flex-col nav:gap-1.5" aria-label="Main">
      <RouterLink
        v-for="item in items"
        :key="item.name"
        :to="item.to"
        custom
        v-slot="{ href, navigate, isActive }"
      >
        <a
          :href="href"
          :aria-current="isActive ? 'page' : undefined"
          :aria-label="item.title"
          class="rounded-inner flex h-12 items-center gap-3.5 px-3 text-[0.9375rem] font-medium whitespace-nowrap nav:px-[0.9375rem]"
          :class="isActive ? 'bg-ink text-white' : 'text-muted hover:bg-hover hover:text-ink'"
          @click="navigate"
        >
          <component :is="item.icon" class="size-5 shrink-0" aria-hidden="true" />
          <span
            class="hidden transition-opacity duration-150 motion-reduce:transition-none nav:inline"
            :class="expanded ? 'opacity-100' : 'opacity-0'"
            >{{ item.title }}</span
          >
        </a>
      </RouterLink>
    </nav>

    <div class="hidden flex-col nav:mt-auto nav:flex">
      <ApiStatus :show-label="expanded" />
    </div>
  </aside>
</template>
