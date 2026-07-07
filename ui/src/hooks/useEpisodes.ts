import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  fetchEpisodes,
  markWatched,
  rewatchEpisode,
  unmarkWatched,
  watchSeason,
  watchThroughEpisode,
} from '../api/tv'
import type { EpisodeWatchState } from '../api/tv'
import { UP_NEXT_KEY } from './useUpNext'

export const episodesKey = (showId: number) => ['tv', 'episodes', showId] as const

/** All episodes of a show with per-episode watched state; only fetched when enabled. */
export function useEpisodes(showId: number, enabled: boolean) {
  return useQuery<EpisodeWatchState[]>({
    queryKey: episodesKey(showId),
    queryFn: () => fetchEpisodes(showId),
    enabled,
  })
}

/**
 * The four episode-picker actions. Each refetches the show's episode list and
 * the Up Next dashboard so both reflect the new watched state.
 */
export function useEpisodeActions(showId: number) {
  const queryClient = useQueryClient()
  const onSuccess = () => {
    void queryClient.invalidateQueries({ queryKey: episodesKey(showId) })
    void queryClient.invalidateQueries({ queryKey: UP_NEXT_KEY })
  }

  const watch = useMutation({ mutationFn: (id: number) => markWatched(id), onSuccess })
  const rewatch = useMutation({ mutationFn: (id: number) => rewatchEpisode(id), onSuccess })
  const watchThrough = useMutation({ mutationFn: (id: number) => watchThroughEpisode(id), onSuccess })
  const unwatch = useMutation({ mutationFn: (id: number) => unmarkWatched(id), onSuccess })
  const markSeason = useMutation({ mutationFn: (season: number) => watchSeason(showId, season), onSuccess })

  return { watch, rewatch, watchThrough, unwatch, markSeason }
}
