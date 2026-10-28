import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { createMemoryHistory, createRouter, type RouteRecordRaw } from 'vue-router'
import { LayoutDashboard, Settings } from '@lucide/vue'

import '@/router/meta'

export const testRoutes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'overview',
    component: { template: '<p>Overview page</p>' },
    meta: {
      title: 'Overview',
      nav: { group: 'main', label: 'Overview', icon: LayoutDashboard, order: 0 },
    },
  },
  {
    path: '/settings',
    name: 'settings',
    component: { template: '<p>Settings page</p>' },
    meta: {
      title: 'Settings',
      nav: { group: 'bottom', label: 'Settings', icon: Settings, order: 0 },
    },
  },
  {
    path: '/hidden',
    name: 'hidden',
    component: { template: '<p>Hidden page</p>' },
    meta: { title: 'Hidden' },
  },
]

/** Plugins every component test needs: PrimeVue, a memory router and a no-retry query client. */
export async function appPlugins(path = '/') {
  const router = createRouter({ history: createMemoryHistory(), routes: testRoutes })
  router.push(path)
  await router.isReady()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return { plugins: [PrimeVue, router, [VueQueryPlugin, { queryClient }]] as never[], router }
}
