<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(defineProps<{ variant?: 'muted' | 'primary' }>(), { variant: 'muted' })
</script>

<template>
  <span
    class="inline-flex items-center rounded-full text-xs px-2.5 py-1"
    :class="variant === 'primary' ? 'bg-primary text-on-primary' : 'bg-surface-muted text-text-soft'"
  >
    <slot />
  </span>
</template>
