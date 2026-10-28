<script setup lang="ts">
import { shallowRef } from 'vue'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import { Menu } from '@lucide/vue'

import AppSidebar from './AppSidebar.vue'

const drawerOpen = shallowRef(false)
</script>

<template>
  <div class="min-h-screen lg:flex">
    <a
      href="#main"
      class="sr-only focus:not-sr-only focus:fixed focus:left-2 focus:top-2 focus:z-50 focus:rounded-md focus:bg-surface-0 focus:px-3 focus:py-2"
    >
      Skip to content
    </a>

    <!-- Desktop: icon rail from 1024px, full sidebar from 1280px. -->
    <aside
      class="sticky top-0 hidden h-screen shrink-0 border-r border-surface-200 bg-surface-0 lg:block lg:w-14 xl:w-60"
    >
      <AppSidebar rail />
    </aside>

    <div class="min-w-0 flex-1">
      <!-- Below 1024px the sidebar becomes a drawer opened from this bar. -->
      <header
        class="flex h-12 items-center gap-2 border-b border-surface-200 bg-surface-0 px-4 lg:hidden"
      >
        <Button
          text
          rounded
          aria-label="Open menu"
          :aria-expanded="drawerOpen"
          @click="drawerOpen = true"
        >
          <template #icon><Menu :size="18" aria-hidden="true" /></template>
        </Button>
        <span class="font-semibold">Mail Sender</span>
      </header>

      <main id="main" tabindex="-1" class="p-6 focus:outline-none">
        <slot />
      </main>
    </div>

    <Drawer v-model:visible="drawerOpen" header="Mail Sender" class="w-64">
      <AppSidebar :brand="false" @navigate="drawerOpen = false" />
    </Drawer>
  </div>
</template>
