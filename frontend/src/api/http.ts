import axios from 'axios'

interface BackendErrorPayload {
  code?: string
  error?: string
}

function resolveDefaultAPIBaseURL() {
  if (typeof window === 'undefined') {
    return 'http://localhost:8081'
  }

  const configured = String(import.meta.env.VITE_API_BASE_URL || '').trim()
  if (configured) {
    return configured
  }

  const protocol = window.location.protocol || 'http:'
  const hostname = window.location.hostname || 'localhost'
  const port = window.location.port || ''

  // 本地开发默认前端 5174、后端 8081；生产环境默认走同源
  if (port === '5174') {
    return `${protocol}//${hostname}:8081`
  }

  return window.location.origin
}

export const apiBaseURL = resolveDefaultAPIBaseURL()

export const http = axios.create({
  baseURL: apiBaseURL,
  timeout: 10_000,
  headers: {
    'Content-Type': 'application/json',
  },
})

let requestInterceptorRegistered = false
let responseInterceptorRegistered = false

export function registerHTTPInterceptors(options: {
  getAccessToken: () => string
  onUnauthorized: (code?: string, message?: string) => void
}) {
  if (!requestInterceptorRegistered) {
    http.interceptors.request.use((config) => {
      const token = String(options.getAccessToken() || '').trim()
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      } else if (config.headers?.Authorization) {
        delete config.headers.Authorization
      }
      return config
    })
    requestInterceptorRegistered = true
  }

  if (!responseInterceptorRegistered) {
    http.interceptors.response.use(
      (response) => response,
      (error) => {
        const status = Number(error?.response?.status || 0)
        if (status === 401) {
          const payload = error?.response?.data as BackendErrorPayload | undefined
          options.onUnauthorized(String(payload?.code || ''), String(payload?.error || ''))
        }
        return Promise.reject(error)
      },
    )
    responseInterceptorRegistered = true
  }
}
