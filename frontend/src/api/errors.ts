import { AxiosError, isAxiosError } from 'axios'

export type ApiErrorKind = 'network' | 'timeout' | 'http'

/** What every failed API call throws, so screens can show the server's own message. */
export class ApiError extends Error {
  readonly status: number
  readonly kind: ApiErrorKind

  constructor(message: string, status: number, kind: ApiErrorKind) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.kind = kind
  }
}

/** Turn anything thrown by axios into an ApiError. Cancellations are passed through untouched. */
export function toApiError(error: unknown): unknown {
  if (error instanceof ApiError) return error
  if (!isAxiosError(error) || error.code === AxiosError.ERR_CANCELED) return error

  if (error.response) {
    const body: unknown = error.response.data
    const serverMessage =
      typeof body === 'object' && body !== null && 'error' in body && typeof body.error === 'string'
        ? body.error
        : ''
    const message = serverMessage || `Request failed (${error.response.status})`
    return new ApiError(message, error.response.status, 'http')
  }
  if (error.code === AxiosError.ECONNABORTED || error.code === AxiosError.ETIMEDOUT) {
    return new ApiError('The server took too long to respond.', 0, 'timeout')
  }
  return new ApiError('Cannot reach the server.', 0, 'network')
}
