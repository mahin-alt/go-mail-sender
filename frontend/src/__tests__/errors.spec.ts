import { AxiosError, type AxiosResponse } from 'axios'
import { describe, expect, it } from 'vitest'

import { ApiError, toApiError } from '@/api/errors'

function httpError(status: number, data: unknown) {
  return new AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    data,
  } as AxiosResponse)
}

describe('toApiError', () => {
  it("uses the server's own message from {error}", () => {
    const err = toApiError(httpError(400, { error: 'smtp host is required' }))
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).message).toBe('smtp host is required')
    expect((err as ApiError).status).toBe(400)
  })

  it('falls back to a generic message when the body has none', () => {
    expect((toApiError(httpError(500, 'oops')) as ApiError).message).toBe('Request failed (500)')
  })

  it('reports an unreachable server', () => {
    const err = toApiError(new AxiosError('Network Error', AxiosError.ERR_NETWORK)) as ApiError
    expect(err.kind).toBe('network')
  })

  it('reports a timeout', () => {
    const err = toApiError(new AxiosError('timeout', AxiosError.ECONNABORTED)) as ApiError
    expect(err.kind).toBe('timeout')
  })

  it('passes cancellations through untouched', () => {
    const cancel = new AxiosError('canceled', AxiosError.ERR_CANCELED)
    expect(toApiError(cancel)).toBe(cancel)
  })
})
