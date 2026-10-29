import type { Component } from 'vue'
import { useRouter } from 'vue-router'

export interface NavItem {
  name: string
  to: string
  title: string
  icon: Component
}

/** The sidebar menu, built from the routes that declare an icon. No grouping, order as listed. */
export function useNavItems(): NavItem[] {
  const router = useRouter()
  return router
    .getRoutes()
    .filter((r) => r.meta.icon && r.meta.title && typeof r.name === 'string')
    .map((r) => ({
      name: r.name as string,
      to: r.path,
      title: r.meta.title as string,
      icon: r.meta.icon as Component,
    }))
}
