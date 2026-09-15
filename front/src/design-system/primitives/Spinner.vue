<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(defineProps<{ size?: 'sm' | 'md' }>(), { size: 'md' })
</script>

<template>
  <span
    role="status"
    aria-label="Загрузка"
    class="inline-block animate-spin rounded-full border-2 border-border border-t-primary"
    :class="size === 'sm' ? 'w-4 h-4' : 'w-6 h-6'"
  />
</template>
