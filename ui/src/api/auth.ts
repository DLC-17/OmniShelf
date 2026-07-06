import { ApiError, request } from './client'

export interface User {
  id: number
  username: string
}

export function login(username: string, password: string): Promise<void> {
  return request<void>('/api/auth/login', {
    method: 'POST',
    body: { username, password },
  })
}

export function register(username: string, password: string, inviteCode: string): Promise<void> {
  return request<void>('/api/auth/register', {
    method: 'POST',
    body: { username, password, inviteCode },
  })
}

export function logout(): Promise<void> {
  return request<void>('/api/auth/logout', { method: 'POST' })
}

/** Returns the current user, or null when not authenticated (401). */
export async function fetchMe(): Promise<User | null> {
  try {
    return await request<User>('/api/auth/me', { skipUnauthorizedHandler: true })
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      return null
    }
    throw err
  }
}
