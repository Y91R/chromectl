import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Badge from './Badge.vue'

const meta = {
  title: 'Design System/Primitives/Badge',
  component: Badge,
  tags: ['autodocs'],
  args: { default: 'бейдж' },
} satisfies Meta<typeof Badge>

export default meta
type Story = StoryObj<typeof meta>

const render = (args: Record<string, unknown>) => ({
  components: { Badge },
  setup: () => ({ args }),
  template: '<Badge v-bind="args">{{ args.default }}</Badge>',
})

export const Muted: Story = { render, args: { variant: 'muted' } }
export const Primary: Story = { render, args: { variant: 'primary' } }
