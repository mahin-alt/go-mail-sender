<script setup lang="ts">
import Toast from 'primevue/toast'

import AppShell from '@/components/layout/AppShell.vue'
</script>

<template>
  <AppShell>
    <RouterView />
  </AppShell>
  <Toast position="bottom-right" />
</template>
