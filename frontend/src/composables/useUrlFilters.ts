import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export const PAGE_SIZE = 20

/**
 * Keeps a list's filters in the address bar (`?status=failed&q=oct&page=2`), so the Back button,
 * a refresh and a copied link all return to the same view. Unknown status values are ignored.
 */
export function useUrlFilters<S extends string>(allowedStatuses: readonly S[]) {
  const route = useRoute()
  const router = useRouter()

  const status = computed<S | ''>(() => {
    const value = route.query.status
    return typeof value === 'string' && (allowedStatuses as readonly string[]).includes(value)
      ? (value as S)
      : ''
  })
  const q = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''))
  const page = computed(() => {
    const n = Number(route.query.page)
    return Number.isInteger(n) && n > 1 ? n : 1
  })
  const offset = computed(() => (page.value - 1) * PAGE_SIZE)

  type Change = { status?: S | ''; q?: string; page?: number }

  /** Changing the status or search goes back to page 1. Defaults are dropped from the URL. */
  function update(change: Change) {
    const next = {
      status: change.status ?? status.value,
      q: change.q ?? q.value,
      page: change.page ?? (change.status !== undefined || change.q !== undefined ? 1 : page.value),
    }
    void router.replace({
      query: {
        ...route.query,
        status: next.status || undefined,
        q: next.q || undefined,
        page: next.page > 1 ? String(next.page) : undefined,
      },
    })
  }

  return { status, q, page, offset, update }
}
