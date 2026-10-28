import { describe, it, expect } from 'vitest'
import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'

import { ApiError, toApiError } from '@/api/errors'

function httpError(status: number, data: unknown) {
  const config = {} as InternalAxiosRequestConfig
  const response = { status, data, statusText: '', headers: {}, config } as AxiosResponse
  return new AxiosError('failed', AxiosError.ERR_BAD_RESPONSE, config, null, response)
}

describe('toApiError', () => {
  it("uses the server's {error} message", () => {
    const err = toApiError(httpError(400, { error: 'subject is required' })) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('subject is required')
    expect(err.status).toBe(400)
    expect(err.kind).toBe('http')
  })

  it('falls back to the status code when the body has no message', () => {
    const err = toApiError(httpError(502, '<html>bad gateway</html>')) as ApiError
    expect(err.message).toBe('Request failed (502)')
  })

  it('reports a network failure when there is no response', () => {
    const err = toApiError(new AxiosError('Network Error', AxiosError.ERR_NETWORK)) as ApiError
    expect(err.kind).toBe('network')
    expect(err.status).toBe(0)
  })

  it('reports a timeout', () => {
    const err = toApiError(new AxiosError('timeout', AxiosError.ECONNABORTED)) as ApiError
    expect(err.kind).toBe('timeout')
  })

  it('passes cancellations and unknown errors through', () => {
    const cancelled = new AxiosError('canceled', AxiosError.ERR_CANCELED)
    expect(toApiError(cancelled)).toBe(cancelled)
    const other = new Error('boom')
    expect(toApiError(other)).toBe(other)
  })
})
