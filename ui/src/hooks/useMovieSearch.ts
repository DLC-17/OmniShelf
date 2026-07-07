import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addMovie, searchMovies } from '../api/movies'
import { LIBRARY_KEY } from './useLibrary'

/** TMDB movie search; disabled until the user submits a non-empty query. */
export function useMovieSearch(query: string) {
  return useQuery({
    queryKey: ['movies', 'search', query] as const,
    queryFn: () => searchMovies(query),
    enabled: query !== '',
  })
}

/**
 * Adds a movie by TMDB id. A fresh add creates a PLAN_TO watchlist item, so the
 * library shelf is refreshed on success (movies never appear in Up Next).
 */
export function useAddMovie() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (tmdbId: number) => addMovie(tmdbId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}
