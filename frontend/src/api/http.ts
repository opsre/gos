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

  // 开发服务器默认直连 8081；生产构建始终走同源，由容器内 Nginx
  // 转发到后端。不能根据浏览器端口判断环境，因为端口映射和外部
  // 反向代理都可能改变公开端口。
  if (import.meta.env.DEV) {
    const protocol = window.location.protocol || 'http:'
    const hostname = window.location.hostname || 'localhost'
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
