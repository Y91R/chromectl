import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Button from './Button.vue'

const meta = {
  title: 'Design System/Primitives/Button',
  component: Button,
  tags: ['autodocs'],
  args: { default: 'Действие' },
} satisfies Meta<typeof Button>

export default meta
type Story = StoryObj<typeof meta>

const render = (args: Record<string, unknown>) => ({
  components: { Button },
  setup: () => ({ args }),
  template: '<Button v-bind="args">{{ args.default }}</Button>',
})

export const Primary: Story = { render, args: { variant: 'primary' } }
export const Secondary: Story = { render, args: { variant: 'secondary' } }
export const Small: Story = { render, args: { size: 'sm' } }
export const Disabled: Story = { render, args: { disabled: true } }
