<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — паттерн дизайн-системы: композиция примитивов -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    label?: string
    hint?: string
    error?: string
    required?: boolean
  }>(),
  { required: false },
)
</script>

<template>
  <label class="flex flex-col gap-tight">
    <span v-if="label" class="text-sm font-medium text-text">
      {{ label }}
      <span v-if="required" class="text-primary">*</span>
    </span>
    <slot />
    <span v-if="error" class="text-xs text-primary">{{ error }}</span>
    <span v-else-if="hint" class="text-xs text-text-muted">{{ hint }}</span>
  </label>
</template>
