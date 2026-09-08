import { request } from './client'

export interface RelatedItem {
  id: number
  type: string
  externalId: string
  title: string
  artworkPath: string
  relationType: 'sequel' | 'prequel' | 'adaptation' | 'same_series' | 'similar' | 'same_universe'
  franchise?: string
  year?: number
  overview?: string
  isTracked: boolean
  userStatus?: string
  userRating?: number
}

export interface GraphNode {
  id: string
  type: string
  title: string
  artworkPath: string
  year?: number
  isTracked: boolean
  userStatus?: string
}

export interface GraphEdge {
  source: string
  target: string
  relationType: string
}

export interface FranchiseGraph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export function fetchRelatedMedia(type: string, externalId: string): Promise<RelatedItem[]> {
  const params = new URLSearchParams({ type, externalId })
  return request<{ items: RelatedItem[] }>(`/api/related?${params.toString()}`).then((r) => r.items)
}

export function fetchFranchiseGraph(type: string, externalId: string): Promise<FranchiseGraph> {
  const params = new URLSearchParams({ type, externalId })
  return request<FranchiseGraph>(`/api/related/graph?${params.toString()}`)
}

export interface RecommendedItem {
  id: number
  type: string
  externalId: string
  title: string
  artworkPath: string
  year?: number
  overview?: string
  isTracked: boolean
  userStatus?: string
  userRating?: number
  matchReason?: string
  score?: number
}

export function fetchRecommendedMedia(type: string, externalId: string): Promise<RecommendedItem[]> {
  const params = new URLSearchParams({ type, externalId })
  return request<{ items: RecommendedItem[] }>(`/api/recommendations?${params.toString()}`).then((r) => r.items)
}

export function dismissRecommendation(type: string, externalId: string): Promise<void> {
  return request<void>('/api/recommendations/reject', {
    method: 'POST',
    body: { type, externalId },
  })
}

export function syncRelatedMedia(): Promise<{ status: string; message: string }> {
  return request<{ status: string; message: string }>('/api/related/sync', { method: 'POST' })
}
