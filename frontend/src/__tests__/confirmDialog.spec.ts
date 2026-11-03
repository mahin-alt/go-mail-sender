import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'

import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

async function open() {
  const wrapper = mount(ConfirmDialog, {
    attachTo: document.body,
    props: { visible: true, title: 'Confirm', message: 'Message', confirmLabel: 'Send' },
    global: { plugins: [PrimeVue] },
  })
  await nextTick()
  await nextTick()
  return wrapper
}

describe('ConfirmDialog', () => {
  it('cannot be dragged around the screen', async () => {
    await open()
    const dialog = document.body.querySelector<HTMLElement>('.p-dialog')!
    const header = dialog.querySelector<HTMLElement>('.p-dialog-header')!
    const before = [dialog.style.left, dialog.style.top, dialog.style.margin]

    header.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, clientX: 10, clientY: 10 }))
    document.dispatchEvent(
      new MouseEvent('mousemove', { bubbles: true, clientX: 200, clientY: 150 }),
    )
    document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))

    // A draggable PrimeVue dialog switches its margin to "0" and starts setting left/top.
    expect([dialog.style.left, dialog.style.top, dialog.style.margin]).toEqual(before)
    expect(document.body.getAttribute('data-p-unselectable-text')).toBeNull()
  })

  it('still has its title, message and both buttons', async () => {
    await open()
    const dialog = document.body.querySelector('.p-dialog')!
    expect(dialog.querySelector('.p-dialog-title')!.textContent).toBe('Confirm')
    expect(dialog.textContent).toContain('Message')
    expect([...dialog.querySelectorAll('button')].map((b) => b.textContent?.trim())).toEqual([
      'Cancel',
      'Send',
    ])
  })
})
