import { http } from './client'
import type { Stats } from '@/types/api'

export interface StatsParams {
  /** Length of the daily series, 1 to 90. */
  days: number
  /** IANA time-zone name. It decides where "today" starts and how emails fall into days. */
  tz: string
}

export async function getStats(params: StatsParams): Promise<Stats> {
  const { data } = await http.get<Stats>('/api/v1/stats', { params })
  return data
}
