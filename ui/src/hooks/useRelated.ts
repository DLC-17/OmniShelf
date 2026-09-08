import { useQuery } from '@tanstack/react-query'
import { fetchFranchiseGraph, fetchRecommendedMedia, fetchRelatedMedia } from '../api/related'
import type { FranchiseGraph, RecommendedItem, RelatedItem } from '../api/related'

export function useRelatedMedia(type: string, externalId: string) {
  return useQuery<RelatedItem[]>({
    queryKey: ['related', type, externalId],
    queryFn: () => fetchRelatedMedia(type, externalId),
    enabled: type !== '' && externalId !== '',
  })
}

export function useFranchiseGraph(type: string, externalId: string) {
  return useQuery<FranchiseGraph>({
    queryKey: ['franchise-graph', type, externalId],
    queryFn: () => fetchFranchiseGraph(type, externalId),
    enabled: type !== '' && externalId !== '',
  })
}

export function useRecommendedMedia(type: string, externalId: string) {
  return useQuery<RecommendedItem[]>({
    queryKey: ['recommendations', type, externalId],
    queryFn: () => fetchRecommendedMedia(type, externalId),
    enabled: type !== '' && externalId !== '',
  })
}
