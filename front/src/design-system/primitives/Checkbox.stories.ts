import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Checkbox from './Checkbox.vue'

const meta = {
  title: 'Design System/Primitives/Checkbox',
  component: Checkbox,
  tags: ['autodocs'],
  args: { label: 'Согласен' },
} satisfies Meta<typeof Checkbox>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
export const Checked: Story = { args: { modelValue: true } }
export const Disabled: Story = { args: { disabled: true } }
