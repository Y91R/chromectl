<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from './api/client'
import ExampleCard from './components/ExampleCard.vue'

const health = ref('…')

onMounted(async () => {
  try {
    const { data } = await api.GET('/health')
    health.value = data?.status ?? 'недоступен'
  } catch {
    health.value = 'недоступен'
  }
})
</script>

<template>
  <div class="min-h-screen bg-neutral-50 flex items-center justify-center p-8">
    <ExampleCard title="chrome_skill" :description="`API: ${health}`" badge="пример" />
  </div>
</template>
