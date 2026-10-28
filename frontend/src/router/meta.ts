import type { Component } from 'vue'

export type NavGroup = 'main' | 'send' | 'prepare' | 'monitor' | 'bottom'

export interface NavMeta {
  group: NavGroup
  label: string
  icon: Component
  order: number
}

declare module 'vue-router' {
  interface RouteMeta {
    /** Page title: used for the browser tab and the page header. */
    title: string
    /** Present only on pages that appear in the sidebar. A page is added here when it works. */
    nav?: NavMeta
  }
}
