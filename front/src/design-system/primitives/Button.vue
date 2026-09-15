<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary'
    size?: 'sm' | 'md'
    type?: 'button' | 'submit' | 'reset'
    disabled?: boolean
  }>(),
  { variant: 'primary', size: 'md', type: 'button', disabled: false },
)
</script>

<template>
  <button
    :type="type"
    :disabled="disabled"
    class="inline-flex items-center justify-center rounded-sm font-medium disabled:opacity-50 disabled:pointer-events-none"
    :class="[
      variant === 'primary'
        ? 'bg-primary text-on-primary hover:bg-primary-hover'
        : 'bg-surface text-text border border-border hover:bg-surface-muted',
      size === 'sm' ? 'text-xs px-3 py-1.5' : 'text-sm px-4 py-2',
    ]"
  >
    <slot />
  </button>
</template>