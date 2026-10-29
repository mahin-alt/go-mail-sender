<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import PageHeader from '@/components/layout/PageHeader.vue'

const route = useRoute()
const title = computed(() => String(route.meta.title ?? ''))
const subtitle = computed(() => `Built in Phase ${route.meta.phase ?? '?'}`)
</script>

<template>
  <PageHeader :title="title" :subtitle="subtitle" />
  <section class="rounded-tile bg-tile p-6">
    <p class="text-muted text-sm">This page is not built yet.</p>
  </section>
</template>
