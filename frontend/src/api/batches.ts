import { http } from './client'
import type {
  BatchProgress,
  BatchStatus,
  BatchSummary,
  Job,
  JobStatus,
  Paged,
  RetryResult,
} from '@/types/api'

export interface BatchListParams {
  status: BatchStatus | ''
  q: string
  limit: number
  offset: number
}

export interface JobListParams {
  status: JobStatus | ''
  q: string
  limit: number
  offset: number
}

/** Empty filters are left out of the query string, as the API expects. */
function clean<T extends object>(params: T): Partial<T> {
  return Object.fromEntries(
    Object.entries(params).filter(([, v]) => v !== '' && v !== undefined),
  ) as Partial<T>
}

export async function listBatches(params: BatchListParams): Promise<Paged<BatchSummary>> {
  const { data } = await http.get<Paged<BatchSummary>>('/api/v1/batches', { params: clean(params) })
  return data
}

export async function getBatch(id: number): Promise<BatchProgress> {
  const { data } = await http.get<BatchProgress>(`/api/v1/batches/${id}`)
  return data
}

/** Always uses the paged form of the endpoint (a `limit` is always sent). */
export async function listJobs(id: number, params: JobListParams): Promise<Paged<Job>> {
  const { data } = await http.get<Paged<Job>>(`/api/v1/batches/${id}/jobs`, {
    params: clean(params),
  })
  return data
}

export async function retryBatch(id: number): Promise<RetryResult> {
  const { data } = await http.post<RetryResult>(`/api/v1/batches/${id}/retry`)
  return data
}

export async function retryJob(id: number): Promise<RetryResult> {
  const { data } = await http.post<RetryResult>(`/api/v1/jobs/${id}/retry`)
  return data
}
