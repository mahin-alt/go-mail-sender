import { computed, shallowRef } from 'vue'

export type SidebarMode = 'open' | 'rail'

const STORAGE_KEY = 'mailflow.sidebar'

function readMode(): SidebarMode {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'rail' ? 'rail' : 'open'
  } catch {
    return 'open' // storage blocked (private window): fall back to the default
  }
}

function saveMode(mode: SidebarMode) {
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    /* the preference just will not be remembered */
  }
}

/**
 * Sidebar behaviour from decision 0007: open by default; the toggle collapses it to an icon rail;
 * while collapsed it opens on hover or keyboard focus and closes again when the pointer leaves.
 * Right after collapsing it stays collapsed until the pointer leaves and comes back (`held`),
 * otherwise nothing would seem to happen because the pointer is still over it.
 */
export function useSidebar() {
  const mode = shallowRef<SidebarMode>(readMode())
  const hovered = shallowRef(false)
  const held = shallowRef(false)
  const keyboardFocus = shallowRef(false)

  const pinned = computed(() => mode.value === 'open')
  const expanded = computed(
    () => pinned.value || keyboardFocus.value || (hovered.value && !held.value),
  )

  function toggle() {
    if (pinned.value) {
      mode.value = 'rail'
      held.value = hovered.value // only matters if the pointer is over it right now
      keyboardFocus.value = false
    } else {
      mode.value = 'open'
      held.value = false
    }
    saveMode(mode.value)
  }

  function onPointerEnter() {
    hovered.value = true
  }

  function onPointerLeave() {
    hovered.value = false
    held.value = false
  }

  /** Only keyboard focus opens the rail; a mouse click that focuses a link must not. */
  function onFocusIn(event: FocusEvent) {
    keyboardFocus.value =
      event.target instanceof HTMLElement && event.target.matches(':focus-visible')
  }

  function onFocusOut(event: FocusEvent) {
    const next = event.relatedTarget
    const inside =
      event.currentTarget instanceof HTMLElement &&
      next instanceof Node &&
      event.currentTarget.contains(next)
    if (!inside) keyboardFocus.value = false
  }

  return { mode, pinned, expanded, toggle, onPointerEnter, onPointerLeave, onFocusIn, onFocusOut }
}
