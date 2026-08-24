import { request } from './client'

export interface SeerrStatus {
  available: boolean
  requested: boolean
}

export async function fetchSeerrConfigured(): Promise<{ configured: boolean }> {
  return request<{ configured: boolean }>('/api/seerr/configured')
}

export async function fetchSeerrStatus(tmdbId: number, type: 'movie' | 'tv'): Promise<SeerrStatus> {
  return request<SeerrStatus>(`/api/seerr/status/${tmdbId}?type=${type}`)
}

export async function requestSeerrMedia(tmdbId: number, type: 'movie' | 'tv'): Promise<{ success: boolean }> {
  return request<{ success: boolean }>('/api/seerr/request', {
    method: 'POST',
    body: { tmdbId, type },
  })
}
