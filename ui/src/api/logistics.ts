import { request } from './client'

export interface ContainerItem {
  itemId: number
  type: string
  title: string
  creator: string
  year?: number
  artworkPath: string
  format?: string
  binderSlot?: number
}

export interface LocationSummary {
  location: string
  containerType: 'SHELF' | 'BINDER' | 'BOX' | 'DRAWER' | 'CABINET' | 'TOTE' | 'OTHER'
  itemCount: number
  mediaBreakdown: Record<string, number>
  filterUrl: string
}

export interface LocationDetail {
  location: string
  containerType: 'SHELF' | 'BINDER' | 'BOX' | 'DRAWER' | 'CABINET' | 'TOTE' | 'OTHER'
  totalItems: number
  items: ContainerItem[]
}

export interface LabelSheetResponse {
  format: string
  svg: string
}

export async function fetchLocations(): Promise<LocationSummary[]> {
  const res = await request<{ locations: LocationSummary[] }>('/api/logistics/locations')
  return res.locations || []
}

export async function fetchLocationDetails(name: string): Promise<LocationDetail> {
  return request<LocationDetail>(`/api/logistics/locations/${encodeURIComponent(name)}`)
}

export function getLabelSheetURL(format: 'avery5160' | 'thermal4x6' | 'thermal2x1', location?: string): string {
  const params = new URLSearchParams({ format, as: 'svg' })
  if (location) {
    params.set('location', location)
  }
  return `/api/logistics/labels?${params.toString()}`
}
