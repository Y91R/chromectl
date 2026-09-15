import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Spinner from './Spinner.vue'

const meta = {
  title: 'Design System/Primitives/Spinner',
  component: Spinner,
  tags: ['autodocs'],
} satisfies Meta<typeof Spinner>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = { args: { size: 'md' } }
export const Small: Story = { args: { size: 'sm' } }
