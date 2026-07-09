import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addGameByIgdb, searchGames } from '../api/games'
import { LIBRARY_KEY } from './useLibrary'

/** IGDB game name search; disabled until the user submits a non-empty query. */
export function useGameSearch(query: string) {
  return useQuery({
    queryKey: ['games', 'search', query] as const,
    queryFn: () => searchGames(query),
    enabled: query !== '',
  })
}

/**
 * Adds a game by IGDB id. A fresh add creates a PLAN_TO watchlist item, so the
 * library shelf is refreshed on success (games never appear in Up Next).
 */
export function useAddGame() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (igdbId: number) => addGameByIgdb(igdbId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}
