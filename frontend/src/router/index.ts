import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { LayoutDashboard } from '@lucide/vue'

import './meta'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'overview',
    component: () => import('@/views/OverviewView.vue'),
    meta: {
      title: 'Overview',
      nav: { group: 'main', label: 'Overview', icon: LayoutDashboard, order: 0 },
    },
  },
]

// Verification page for the stack, only exists on the dev server.
if (import.meta.env.DEV) {
  routes.push({
    path: '/dev/stack-check',
    name: 'stack-check',
    component: () => import('@/views/StackCheckView.vue'),
    meta: { title: 'Stack check' },
  })
}

routes.push({
  path: '/:pathMatch(.*)*',
  name: 'not-found',
  component: () => import('@/views/NotFoundView.vue'),
  meta: { title: 'Page not found' },
})

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.afterEach((to) => {
  document.title = `${to.meta.title} · Mail Sender`
})

export default router
