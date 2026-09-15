import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Stack from './Stack.vue'
import { Badge } from '../primitives'

const meta = {
  title: 'Design System/Layout/Stack',
  component: Stack,
  tags: ['autodocs'],
} satisfies Meta<typeof Stack>

export default meta
type Story = StoryObj<typeof meta>

const render = (args: Record<string, unknown>) => ({
  components: { Stack, Badge },
  setup: () => ({ args }),
  template: `
    <Stack v-bind="args">
      <Badge>один</Badge>
      <Badge>два</Badge>
      <Badge>три</Badge>
    </Stack>
  `,
})

export const Column: Story = { render, args: { direction: 'col', align: 'start' } }
export const Row: Story = { render, args: { direction: 'row', gap: 'card' } }
