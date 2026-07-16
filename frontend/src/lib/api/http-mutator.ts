import Axios from 'axios'
import type {
  AxiosError,
  AxiosRequestConfig,
  InternalAxiosRequestConfig,
} from 'axios'
import { getAccessToken, clearSession } from '#/lib/auth/auth-session'
import { refreshSession, apiBaseURL } from '#/lib/auth/auth-api'
import { toApiError } from './api-error'

// Domain API client → Traefik/Go with Bearer AT (not cookies).
export const axiosInstance = Axios.create({
  baseURL: apiBaseURL(),
})

axiosInstance.interceptors.request.use((config) => {
  const token = getAccessToken()
  if (token) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  // X-User-Id is injected by Traefik ForwardAuth. Browser must never send it.
  config.headers.delete('X-User-Id')
  return config
})

let refreshPromise: Promise<void> | null = null

async function singleFlightRefresh(): Promise<void> {
  if (!refreshPromise) {
    refreshPromise = refreshSession()
      .then(() => undefined)
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

axiosInstance.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const original = error.config as
      | (InternalAxiosRequestConfig & { _retry?: boolean })
      | undefined
    if (
      !original ||
      error.response?.status !== 401 ||
      original._retry
    ) {
      throw toApiError(error)
    }

    original._retry = true
    try {
      await singleFlightRefresh()
      const token = getAccessToken()
      if (token) {
        original.headers.set('Authorization', `Bearer ${token}`)
      }
      return axiosInstance(original)
    } catch (refreshError) {
      clearSession()
      throw toApiError(refreshError)
    }
  },
)

/**
 * Orval mutator. Generated hooks call this; returns response body and throws
 * normalized ApiError on failure.
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

  // @ts-expect-error attach cancel for Orval query cancellation
  promise.cancel = () => source.cancel('Query was cancelled')

  return promise
}

export default apiClient
