import type { Component } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { LayoutGrid, Mail, MailCheck, Send, Settings, TriangleAlert } from '@lucide/vue'

declare module 'vue-router' {
  interface RouteMeta {
    /** Page name, used for the header, the sidebar label and the browser tab title. */
    title?: string
    /** When set, the route appears in the sidebar (in the order the routes are listed). */
    icon?: Component
    /** Which roadmap phase builds the real page; shown by the placeholder until then. */
    phase?: number
  }
}

const Placeholder = () => import('@/views/PlaceholderView.vue')

/** Sidebar order: Dashboard, Bulk Emails, Single Email, Verify Emails, Failures, Settings. */
export const routes: RouteRecordRaw[] = [
  { path: '/', redirect: { name: 'dashboard' } },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: Placeholder,
    meta: { title: 'Dashboard', icon: LayoutGrid, phase: 5 },
  },
  {
    path: '/bulk-emails',
    name: 'bulk-emails',
    component: () => import('@/views/BulkEmailsView.vue'),
    meta: { title: 'Bulk Emails', icon: Mail },
  },
  {
    path: '/bulk-emails/new',
    name: 'new-bulk-email',
    component: Placeholder,
    meta: { title: 'New Bulk Email', phase: 3 },
  },
  {
    path: '/bulk-emails/:id(\\d+)',
    name: 'bulk-email-detail',
    component: () => import('@/views/BulkEmailDetailView.vue'),
    meta: { title: 'Bulk Email' },
  },
  {
    path: '/single-email',
    name: 'single-email',
    component: Placeholder,
    meta: { title: 'Single Email', icon: Send, phase: 6 },
  },
  {
    path: '/verify-emails',
    name: 'verify-emails',
    component: Placeholder,
    meta: { title: 'Verify Emails', icon: MailCheck, phase: 6 },
  },
  {
    path: '/failures',
    name: 'failures',
    component: Placeholder,
    meta: { title: 'Failures', icon: TriangleAlert, phase: 7 },
  },
  {
    path: '/settings',
    name: 'settings',
    component: Placeholder,
    meta: { title: 'Settings', icon: Settings, phase: 4 },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { title: 'Page not found' },
  },
]
