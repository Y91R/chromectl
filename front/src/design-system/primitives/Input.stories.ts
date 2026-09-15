import type { Meta, StoryObj } from '@storybook/vue3-vite'
import Input from './Input.vue'

const meta = {
  title: 'Design System/Primitives/Input',
  component: Input,
  tags: ['autodocs'],
  args: { placeholder: 'Введите текст' },
} satisfies Meta<typeof Input>

export default meta
type Story = StoryObj<typeof meta>

export const Default: Story = {}
export const Filled: Story = { args: { modelValue: 'Значение' } }
export const Disabled: Story = { args: { disabled: true } }
export const Invalid: Story = { args: { invalid: true, modelValue: 'Ошибка' } }
