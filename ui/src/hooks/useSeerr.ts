import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchSeerrConfigured, fetchSeerrStatus, requestSeerrMedia } from '../api/seerr'

export const SEERR_CONFIGURED_KEY = ['seerr', 'configured'] as const
export const SEERR_STATUS_KEY = (tmdbId: number, type: string) => ['seerr', 'status', tmdbId, type] as const

export function useSeerrConfigured() {
  return useQuery({
    queryKey: SEERR_CONFIGURED_KEY,
    queryFn: async () => {
      const res = await fetchSeerrConfigured()
      return res.configured
    },
  })
}

export function useSeerrStatus(tmdbId: number, type: 'movie' | 'tv', enabled: boolean) {
  return useQuery({
    queryKey: SEERR_STATUS_KEY(tmdbId, type),
    queryFn: () => fetchSeerrStatus(tmdbId, type),
    enabled,
  })
}

export function useRequestSeerrMedia() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tmdbId, type }: { tmdbId: number; type: 'movie' | 'tv' }) => requestSeerrMedia(tmdbId, type),
    onSuccess: (_, { tmdbId, type }) => {
      queryClient.invalidateQueries({ queryKey: SEERR_STATUS_KEY(tmdbId, type) })
    },
  })
}
