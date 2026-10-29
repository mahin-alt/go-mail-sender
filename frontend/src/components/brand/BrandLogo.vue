<script setup lang="ts">
/** Paper plane mark + wordmark (decision 0007). The wordmark hides while the sidebar is a rail. */
defineProps<{ showWordmark?: boolean }>()
</script>

<template>
  <div class="flex h-10 items-center gap-3 px-2 whitespace-nowrap">
    <svg
      class="text-brand shrink-0"
      width="32"
      height="32"
      viewBox="0 0 32 32"
      fill="none"
      aria-hidden="true"
    >
      <g stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">
        <path d="M28 4 4 13l9 4 4 9z" />
        <path d="M28 4 13 17" />
      </g>
    </svg>
    <span
      class="hidden text-xl tracking-tight transition-opacity duration-150 motion-reduce:transition-none nav:inline"
      :class="showWordmark ? 'opacity-100' : 'opacity-0'"
      ><b class="font-extrabold">Mail</b><span class="font-light">Flow</span></span
    >
    <span class="sr-only nav:hidden">MailFlow</span>
  </div>
</template>
