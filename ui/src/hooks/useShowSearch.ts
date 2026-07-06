import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addShow, searchShows } from '../api/tv'
import { LIBRARY_KEY } from './useLibrary'
import { UP_NEXT_KEY } from './useUpNext'

/** TMDB show search; disabled until the user submits a non-empty query. */
export function useShowSearch(query: string) {
  return useQuery({
    queryKey: ['tv', 'search', query] as const,
    queryFn: () => searchShows(query),
    enabled: query !== '',
  })
}

/**
 * Adds a show by TMDB id (spec §2.2 step 2). A fresh add creates a WATCHING
 * item, so both Up Next and the library shelf are refreshed on success.
 */
export function useAddShow() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (tmdbId: number) => addShow(tmdbId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: UP_NEXT_KEY }),
        queryClient.invalidateQueries({ queryKey: LIBRARY_KEY }),
      ])
    },
  })
}
