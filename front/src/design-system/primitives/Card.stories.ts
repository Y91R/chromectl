import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Card from './Card.vue'

const meta = {
  title: 'Design System/Primitives/Card',
  component: Card,
  tags: ['autodocs'],
} satisfies Meta<typeof Card>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  render: () => ({
    components: { Card },
    template: `
      <Card class="max-w-sm">
        <p class="text-sm text-text">Контент карточки на токенах дизайн-системы.</p>
      </Card>
    `,
  }),
}

export const WithHeaderAndFooter: Story = {
  render: () => ({
    components: { Card },
    template: `
      <Card class="max-w-sm">
        <template #header><h2 class="text-sm font-medium text-text">Заголовок</h2></template>
        <p class="text-sm text-text-muted">Тело карточки.</p>
        <template #footer><span class="text-xs text-text-muted">Подвал</span></template>
      </Card>
    `,
  }),
}
