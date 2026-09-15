import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Select from './Select.vue'

const meta = {
  title: 'Design System/Primitives/Select',
  component: Select,
  tags: ['autodocs'],
  args: {
    options: [
      { label: 'Первый', value: '1' },
      { label: 'Второй', value: '2' },
      { label: 'Третий', value: '3' },
    ],
  },
} satisfies Meta<typeof Select>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
export const Disabled: Story = { args: { disabled: true } }
