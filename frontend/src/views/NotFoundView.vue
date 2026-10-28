<script setup lang="ts">
import PageHeader from '@/components/layout/PageHeader.vue'
</script>

<template>
  <PageHeader title="Page not found" />
  <p class="text-surface-600">
    There is nothing at this address.
    <RouterLink class="text-primary underline" to="/">Go to Overview</RouterLink>.
  </p>
</template>
