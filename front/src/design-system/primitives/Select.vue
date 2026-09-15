<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: string
    options: { label: string; value: string }[]
    disabled?: boolean
  }>(),
  { modelValue: '', disabled: false },
)
defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <select
    :value="modelValue"
    :disabled="disabled"
    class="w-full bg-surface text-text text-sm rounded-sm border border-border px-3 py-2 focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
    @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
  </select>
</template>
