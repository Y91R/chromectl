import type { Meta, StoryObj } from '@storybook/vue3-vite'
import FormField from './FormField.vue'
import { Input } from '../primitives'

const meta = {
  title: 'Design System/Patterns/FormField',
  component: FormField,
  tags: ['autodocs'],
} satisfies Meta<typeof FormField>

export default meta
type Story = StoryObj<typeof meta>

const render = (args: Record<string, unknown>) => ({
  components: { FormField, Input },
  setup: () => ({ args }),
  template: `
    <FormField v-bind="args" class="max-w-sm">
      <Input placeholder="you@example.com" :invalid="!!args.error" />
    </FormField>
  `,
})

export const WithHint: Story = {
  render,
  args: { label: 'Email', hint: 'Используется для входа', required: true },
}

export const WithError: Story = {
  render,
  args: { label: 'Email', error: 'Некорректный адрес', required: true },
}

export const LabelOnly: Story = {
  render,
  args: { label: 'Имя' },
}
