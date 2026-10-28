<script setup lang="ts">
const isDev = import.meta.env.DEV
</script>

<template>
  <main class="mx-auto max-w-3xl p-6">
    <h1 class="text-xl font-semibold">Mail Sender dashboard</h1>
    <p class="mt-2 text-surface-600">Scaffold is running. Pages arrive in the next phases.</p>
    <RouterLink v-if="isDev" class="mt-4 inline-block text-primary underline" to="/dev/stack-check">
      Open the stack check page
    </RouterLink>
  </main>
</template>
