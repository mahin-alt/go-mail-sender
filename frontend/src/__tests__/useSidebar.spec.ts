import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useSidebar } from '@/composables/useSidebar'

describe('useSidebar', () => {
  beforeEach(() => localStorage.clear())

  it('is open and pinned by default', () => {
    const s = useSidebar()
    expect(s.pinned.value).toBe(true)
    expect(s.expanded.value).toBe(true)
  })

  it('collapses on toggle and stays collapsed while the pointer is still over it', () => {
    const s = useSidebar()
    s.onPointerEnter()
    s.toggle()
    expect(s.pinned.value).toBe(false)
    expect(s.expanded.value).toBe(false)
  })

  it('opens on the first hover when it was collapsed without the pointer over it', () => {
    const s = useSidebar()
    s.toggle() // e.g. from the keyboard
    s.onPointerEnter()
    expect(s.expanded.value).toBe(true)
  })

  it('opens on hover after the pointer left and came back, and closes when it leaves', () => {
    const s = useSidebar()
    s.toggle()
    s.onPointerLeave()
    s.onPointerEnter()
    expect(s.expanded.value).toBe(true)
    s.onPointerLeave()
    expect(s.expanded.value).toBe(false)
  })

  it('pins open again from the collapsed state', () => {
    const s = useSidebar()
    s.toggle()
    s.onPointerEnter()
    s.toggle()
    expect(s.pinned.value).toBe(true)
    s.onPointerLeave()
    expect(s.expanded.value).toBe(true)
  })

  it('remembers the choice', () => {
    useSidebar().toggle()
    expect(useSidebar().pinned.value).toBe(false)
  })

  it('still works when storage is blocked', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const s = useSidebar()
    expect(s.pinned.value).toBe(true)
    expect(() => s.toggle()).not.toThrow()
    vi.restoreAllMocks()
  })
})
