import { http } from './client'
import type { Health } from '@/types/api'

export async function getHealth(): Promise<Health> {
  const { data } = await http.get<Health>('/healthz')
  return data
}
