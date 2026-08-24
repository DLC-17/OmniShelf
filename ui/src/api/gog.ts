import { request } from './client'

export interface GOGStatus {
  connected: boolean
  username: string
  source: 'oauth' | 'env' | 'none'
  lastSyncedAt: string | null
}

export interface GOGAuthUrlResponse {
  authUrl: string
}

export interface GOGConnectResponse {
  connected?: boolean
  username?: string
  source?: string
  message?: string
  twoFactorRequired?: boolean
}

export interface GOGSyncResponse {
  success: boolean
  importedCount: number
  totalOwned: number
  lastSyncedAt: string
  message: string
}

/** Check whether GOG is connected for the authenticated user. */
export function getGOGStatus(): Promise<GOGStatus> {
  return request<GOGStatus>('/api/gog/status')
}

/** Direct in-app sign in with GOG email and password (automatically captures token without manual code pasting). */
export function directGOGLogin(payload: { email: string; password: string; twoFactorCode?: string }): Promise<GOGConnectResponse> {
  return request<GOGConnectResponse>('/api/gog/login', {
    method: 'POST',
    body: payload,
  })
}

/** Get the official GOG OAuth login URL. */
export function getGOGAuthURL(): Promise<GOGAuthUrlResponse> {
  return request<GOGAuthUrlResponse>('/api/gog/auth-url')
}

/** Connect GOG account via OAuth authorization code or manual access token. */
export function connectGOG(payload: { code?: string; accessToken?: string }): Promise<GOGConnectResponse> {
  return request<GOGConnectResponse>('/api/gog/connect', {
    method: 'POST',
    body: payload,
  })
}

/** Trigger DRM-free game library synchronization from GOG. */
export function syncGOGLibrary(): Promise<GOGSyncResponse> {
  return request<GOGSyncResponse>('/api/gog/sync', {
    method: 'POST',
  })
}

/** Disconnect linked GOG account. */
export function disconnectGOG(): Promise<{ success: boolean; message: string }> {
  return request<{ success: boolean; message: string }>('/api/gog/disconnect', {
    method: 'DELETE',
  })
}
