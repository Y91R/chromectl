import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Input from './Input.vue'

describe('Input', () => {
  it('эмитит update:modelValue при вводе', async () => {
    const w = mount(Input)
    await w.get('input').setValue('привет')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['привет'])
  })

  it('invalid=true ставит aria-invalid', () => {
    const w = mount(Input, { props: { invalid: true } })
    expect(w.get('input').attributes('aria-invalid')).toBe('true')
  })
})
