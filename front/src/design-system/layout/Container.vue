<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — layout-примитив дизайн-системы: центрированная колонка контента -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'md' },
)
</script>

<template>
  <div
    class="mx-auto w-full px-card"
    :class="size === 'sm' ? 'max-w-md' : size === 'lg' ? 'max-w-5xl' : 'max-w-2xl'"
  >
    <slot />
  </div>
</template>
