import type { Meta, StoryObj } from '@storybook/vue3-vite'
import ExampleCard from './ExampleCard.vue'

const meta = {
  title: 'Example/ExampleCard',
  component: ExampleCard,
  tags: ['autodocs'],
} satisfies Meta<typeof ExampleCard>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {
  args: {
    title: 'Пример карточки',
    description: 'Компонент-образец: story собирается на моках, проверяется скриншотом через browser MCP и только потом подключается к API.',
    badge: 'пример',
  },
}

export const TitleOnly: Story = {
  args: {
    title: 'Только заголовок',
  },
}
