<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary'
    /** `lg` is a slightly bigger button, for the one main action of a page. */
    size?: 'md' | 'lg'
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit'
  }>(),
  { variant: 'primary', size: 'md', loading: false, disabled: false, type: 'button' },
)
</script>

<template>
  <button
    :type="type"
    class="inline-flex cursor-pointer items-center justify-center gap-2 rounded-full font-semibold disabled:cursor-default disabled:opacity-40"
    :class="[
      size === 'lg' ? 'px-6 py-3 text-base' : 'px-5 py-2.5 text-sm',
      variant === 'primary'
        ? 'bg-ink text-white hover:bg-black'
        : 'bg-pill text-ink hover:bg-track',
    ]"
    :disabled="disabled || loading"
    :aria-busy="loading"
  >
    <span
      v-if="loading"
      class="size-3.5 animate-spin rounded-full border-2 border-current border-t-transparent motion-reduce:animate-none"
      aria-hidden="true"
    />
    <slot />
  </button>
</template>
