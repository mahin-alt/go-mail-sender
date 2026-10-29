/**
 * How often progress screens re-ask the API (decision 0010: always-on polling).
 * Change here to tune; switching to "only while active" later means returning `false` from
 * `refetchInterval` when nothing is pending or processing.
 */
export const POLL_MS = {
  list: 5_000,
  detail: 3_000,
} as const
