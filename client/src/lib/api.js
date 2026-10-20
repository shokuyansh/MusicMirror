export const API_BASE = (import.meta.env.VITE_API_URL || '').replace(/\/$/, '')

export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.status = status
  }
}

export async function request(
  path,
  { token, method = 'GET', body, signal } = {},
) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`
  const isForm = body instanceof FormData
  if (body !== undefined && !isForm)
    headers['Content-Type'] = 'application/json'
  let response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body:
        body === undefined ? undefined : isForm ? body : JSON.stringify(body),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(90000)])
        : AbortSignal.timeout(90000),
    })
  } catch (error) {
    if (error.name === 'AbortError') throw error
    throw new ApiError(
      'Cannot reach the server. It may be waking up; please retry in a moment.',
      0,
    )
  }
  if (response.status === 204) return null
  const data = await response.json().catch(() => null)
  if (!response.ok)
    throw new ApiError(
      data?.error || 'The server is temporarily unavailable. Please retry.',
      response.status,
    )
  if (!data)
    throw new ApiError(
      'The server is waking up. Please retry in a moment.',
      503,
    )
  return data
}

export function websocketURL() {
  const url = new URL(`${API_BASE}/api/ws`, window.location.origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.href
}

export function audioURL(url) {
  return new URL(url, API_BASE || window.location.origin).href
}
