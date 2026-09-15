<!-- Бизнес-компонент: собран из дизайн-системы (front/src/design-system). -->
<script setup lang="ts">
import { Card, Badge, Button } from '../design-system'

defineProps<{
  title: string
  description?: string
  badge?: string
}>()
</script>

<template>
  <Card class="w-full max-w-sm">
    <template #header>
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-sm font-medium text-text">{{ title }}</h2>
        <Badge v-if="badge">{{ badge }}</Badge>
      </div>
    </template>

    <p v-if="description" class="text-sm text-text-muted">{{ description }}</p>

    <template #footer>
      <Button>Действие</Button>
    </template>
  </Card>
</template>
