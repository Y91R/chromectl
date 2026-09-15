<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — layout-примитив дизайн-системы на токенах spacing -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    direction?: 'col' | 'row'
    gap?: 'tight' | 'card'
    align?: 'start' | 'center' | 'stretch'
  }>(),
  { direction: 'col', gap: 'tight', align: 'stretch' },
)
</script>

<template>
  <div
    class="flex"
    :class="[
      direction === 'col' ? 'flex-col' : 'flex-row',
      gap === 'card' ? 'gap-card' : 'gap-tight',
      align === 'start' ? 'items-start' : align === 'center' ? 'items-center' : 'items-stretch',
    ]"
  >
    <slot />
  </div>
</template>
