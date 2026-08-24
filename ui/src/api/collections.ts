import { request } from './client'

export interface Collection {
  id: number
  name: string
  slug: string
  createdAt: string
  updatedAt: string
}

export function getCollections(): Promise<Collection[]> {
  return request<Collection[]>('/api/collections')
}

export function createCollection(name: string, slug: string): Promise<Collection> {
  return request<Collection>('/api/collections', {
    method: 'POST',
    body: { name, slug },
  })
}

export function updateCollection(id: number, name: string, slug: string): Promise<Collection> {
  return request<Collection>(`/api/collections/${id}`, {
    method: 'PUT',
    body: { name, slug },
  })
}

export function deleteCollection(id: number): Promise<void> {
  return request<void>(`/api/collections/${id}`, {
    method: 'DELETE',
  })
}

export function addCollectionItem(id: number, itemId: number): Promise<{ success: boolean }> {
  return request<{ success: boolean }>(`/api/collections/${id}/items`, {
    method: 'POST',
    body: { itemId },
  })
}

export function removeCollectionItem(id: number, itemId: number): Promise<void> {
  return request<void>(`/api/collections/${id}/items/${itemId}`, {
    method: 'DELETE',
  })
}
