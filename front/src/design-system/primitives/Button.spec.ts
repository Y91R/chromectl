import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Button from './Button.vue'

describe('Button', () => {
  it('рендерит слот и primary-токены по умолчанию', () => {
    const w = mount(Button, { slots: { default: 'OK' } })
    expect(w.text()).toBe('OK')
    expect(w.classes()).toContain('bg-primary')
    expect(w.classes()).toContain('text-on-primary')
  })

  it('variant=secondary переключает токены поверхности', () => {
    const w = mount(Button, { props: { variant: 'secondary' } })
    expect(w.classes()).toContain('bg-surface')
    expect(w.classes()).not.toContain('bg-primary')
  })

  it('disabled выставляет атрибут', () => {
    const w = mount(Button, { props: { disabled: true } })
    expect(w.attributes('disabled')).toBeDefined()
  })
})
