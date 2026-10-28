import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue'),
  },
]

// Phase 0 verification page, only exists in the dev server. Remove or keep as a dev aid.
if (import.meta.env.DEV) {
  routes.push({
    path: '/dev/stack-check',
    name: 'stack-check',
    component: () => import('@/views/StackCheckView.vue'),
  })
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

export default router
