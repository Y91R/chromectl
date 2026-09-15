<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
import { useSlots } from 'vue'
const slots = useSlots()
</script>

<template>
  <div class="bg-surface rounded-md border border-border overflow-hidden">
    <div v-if="slots.header" class="p-4 border-b border-border">
      <slot name="header" />
    </div>
    <div class="p-4">
      <slot />
    </div>
    <div v-if="slots.footer" class="p-3 border-t border-border">
      <slot name="footer" />
    </div>
  </div>
</template>
