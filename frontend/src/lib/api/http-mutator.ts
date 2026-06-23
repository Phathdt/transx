import Axios from 'axios'
import type { AxiosRequestConfig } from 'axios'
import { getAccessToken } from '#/lib/auth/auth-session'
import { toApiError } from './api-error'

// Browser calls the same-origin /api/v1 prefix by default; vite proxies it to
// the Traefik gateway in dev. Override with VITE_API_BASE_URL if needed.
const baseURL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export const axiosInstance = Axios.create({ baseURL })

axiosInstance.interceptors.request.use((config) => {
  const token = getAccessToken()
  if (token) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  // X-User-Id is injected by the Traefik ForwardAuth gateway after it validates
  // the bearer token via /check. The browser must never send it.
  config.headers.delete('X-User-Id')
  return config
})

/**
 * Orval mutator. Generated hooks call this; it returns the response body and
 * throws a normalized ApiError on failure. Supports request cancellation via
 * the returned promise's `.cancel`, matching Orval's expected signature.
 */
export const apiClient = <T>(
  config: AxiosRequestConfig,
  options?: AxiosRequestConfig,
): Promise<T> => {
  const source = Axios.CancelToken.source()
  const promise = axiosInstance({
    ...config,
    ...options,
    cancelToken: source.token,
  })
    .then(({ data }) => data as T)
    .catch((error) => {
      throw toApiError(error)
    })

  // @ts-expect-error attach cancel for Orval's query cancellation support
  promise.cancel = () => source.cancel('Query was cancelled')

  return promise
}

export default apiClient
