<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(
  defineProps<{ modelValue?: boolean; label?: string; disabled?: boolean }>(),
  { modelValue: false, disabled: false },
)
defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <label class="inline-flex items-center gap-2 text-sm text-text" :class="disabled && 'opacity-50'">
    <input
      type="checkbox"
      class="accent-primary"
      :checked="modelValue"
      :disabled="disabled"
      @change="$emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
    />
    <span v-if="label">{{ label }}</span>
  </label>
</template>
