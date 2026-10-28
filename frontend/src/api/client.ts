import axios from 'axios'

import { toApiError } from './errors'

/**
 * The only axios instance. The browser calls the Go API directly (CORS), so the origin serving
 * this app must be in the API's CORS_ALLOWED_ORIGINS. Only src/api and src/queries import this.
 */
export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || 'http://localhost:8080',
  timeout: 30_000,
})

http.interceptors.response.use(
  (response) => response,
  (error: unknown) => Promise.reject(toApiError(error)),
)
