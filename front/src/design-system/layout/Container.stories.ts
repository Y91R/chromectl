import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Container from './Container.vue'

const meta = {
  title: 'Design System/Layout/Container',
  component: Container,
  tags: ['autodocs'],
} satisfies Meta<typeof Container>

export default meta
type Story = StoryObj<typeof meta>

const render = (args: Record<string, unknown>) => ({
  components: { Container },
  setup: () => ({ args }),
  template: `
    <Container v-bind="args">
      <div class="bg-surface border border-border rounded-md p-card text-sm text-text">
        Контент в центрированной колонке.
      </div>
    </Container>
  `,
})

export const Medium: Story = { render, args: { size: 'md' } }
export const Small: Story = { render, args: { size: 'sm' } }
