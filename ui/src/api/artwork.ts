import { request, requestUpload } from './client'

/** Response of both artwork endpoints: the new relative /images path. */
export interface ArtworkResponse {
  artworkPath: string
}

/** Re-pull the latest cover art for a tracked item from its upstream source. */
export function refreshArtwork(itemId: number): Promise<ArtworkResponse> {
  return request<ArtworkResponse>(`/api/items/${itemId}/artwork/refresh`, { method: 'POST' })
}

/** Replace a tracked item's cover with a user-supplied image file. */
export function uploadArtwork(itemId: number, file: File): Promise<ArtworkResponse> {
  const form = new FormData()
  form.append('image', file)
  return requestUpload<ArtworkResponse>(`/api/items/${itemId}/artwork`, form, 'PUT')
}
