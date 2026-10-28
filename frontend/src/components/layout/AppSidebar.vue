<script setup lang="ts">
import ApiStatus from './ApiStatus.vue'
import { useNavItems } from '@/composables/useNavItems'

const { brand = true } = defineProps<{
  /** Desktop icon rail between lg and xl: labels are visually hidden but still announced. */
  rail?: boolean
  /** Show the app name on top (the mobile drawer already has it in its header). */
  brand?: boolean
}>()

const emit = defineEmits<{ navigate: [] }>()

const { sections } = useNavItems()
</script>

<template>
  <div class="flex h-full flex-col gap-4 p-2">
    <p
      v-if="brand"
      class="px-2 pt-1 text-base font-semibold"
      :class="{ 'sr-only xl:not-sr-only': rail }"
    >
      Mail Sender
    </p>

    <nav aria-label="Main" class="flex flex-1 flex-col gap-4">
      <section
        v-for="section in sections"
        :key="section.group"
        :class="{ 'mt-auto': section.group === 'bottom' }"
      >
        <h2
          v-if="section.heading"
          class="mb-1 px-2 text-xs font-semibold uppercase tracking-wide text-surface-600"
          :class="{ 'sr-only xl:not-sr-only': rail }"
        >
          {{ section.heading }}
        </h2>
        <ul class="flex flex-col gap-0.5">
          <li v-for="item in section.items" :key="item.name">
            <RouterLink
              :to="item.to"
              :title="item.label"
              class="flex h-9 items-center gap-3 rounded-md px-2 text-surface-700 hover:bg-surface-100 aria-[current=page]:bg-primary-50 aria-[current=page]:font-medium aria-[current=page]:text-primary-700"
              @click="emit('navigate')"
            >
              <component :is="item.icon" :size="18" aria-hidden="true" class="shrink-0" />
              <span :class="{ 'sr-only xl:not-sr-only': rail }">{{ item.label }}</span>
            </RouterLink>
          </li>
        </ul>
      </section>
    </nav>

    <div class="border-t border-surface-200 px-2 pt-3">
      <ApiStatus :rail="rail" />
    </div>
  </div>
</template>
