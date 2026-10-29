<script setup lang="ts">
import PageHeader from '@/components/layout/PageHeader.vue'
</script>

<template>
  <PageHeader title="Page not found" subtitle="That address does not exist." />
  <section class="rounded-tile bg-tile p-6">
    <RouterLink :to="{ name: 'dashboard' }" class="font-semibold underline"
      >Go to the Dashboard</RouterLink
    >
  </section>
</template>
