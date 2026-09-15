<!-- ADR: docs/adr/0004-dizayn-sistema-sloi.md — примитив дизайн-системы на токенах DESIGN.md -->
<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    type?: string
    disabled?: boolean
    invalid?: boolean
  }>(),
  { modelValue: '', type: 'text', disabled: false, invalid: false },
)
defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <input
    :type="type"
    :value="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :aria-invalid="invalid"
    class="w-full bg-surface text-text text-sm rounded-sm border px-3 py-2 placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-primary disabled:opacity-50"
    :class="invalid ? 'border-primary' : 'border-border'"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
