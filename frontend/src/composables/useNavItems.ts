import { computed } from 'vue'
import { useRouter } from 'vue-router'

import type { NavGroup, NavMeta } from '@/router/meta'

export interface NavItem extends NavMeta {
  to: string
  name: string
}

export interface NavSection {
  group: NavGroup
  /** Heading shown above the items; the main and bottom groups have none. */
  heading?: string
  items: NavItem[]
}

const SECTIONS: { group: NavGroup; heading?: string }[] = [
  { group: 'main' },
  { group: 'send', heading: 'Send' },
  { group: 'prepare', heading: 'Prepare' },
  { group: 'monitor', heading: 'Monitor' },
  { group: 'bottom' },
]

/** Sidebar content, built from route meta so a page appears in the menu only once it exists. */
export function useNavItems() {
  const router = useRouter()

  const sections = computed<NavSection[]>(() => {
    const items: NavItem[] = router
      .getRoutes()
      .filter((route) => route.meta.nav && typeof route.name === 'string')
      .map((route) => ({ ...route.meta.nav!, to: route.path, name: route.name as string }))

    return SECTIONS.map(({ group, heading }) => ({
      group,
      heading,
      items: items.filter((item) => item.group === group).sort((a, b) => a.order - b.order),
    })).filter((section) => section.items.length > 0)
  })

  return { sections }
}
