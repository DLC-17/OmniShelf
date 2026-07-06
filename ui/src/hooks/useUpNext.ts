import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchUpNext, markWatched } from '../api/tv'
import type { UpNextEntry, UpNextFilter } from '../api/tv'

/** Prefix key for every recency-filtered Up Next query. */
export const UP_NEXT_KEY = ['tv', 'up-next'] as const
export const upNextKey = (filter: UpNextFilter) => ['tv', 'up-next', filter] as const

/**
 * Cache entry for the Up Next dashboard. `optimisticWatched` is a client-only
 * flag set while a mark-watched request is in flight so the checkmark fills in
 * immediately; it is rolled back on error and reconciled by a refetch.
 */
export interface UpNextCard extends UpNextEntry {
  optimisticWatched?: boolean
}

export function useUpNext(filter: UpNextFilter) {
  return useQuery<UpNextCard[]>({
    queryKey: upNextKey(filter),
    queryFn: () => fetchUpNext(filter),
  })
}

/**
 * One-tap watch checkmark. Optimistically fills the checkmark across every
 * cached recency bucket, rolls back on error, and on settle refetches so the
 * server re-buckets the show (e.g. a just-watched show moves into "recent").
 */
export function useMarkWatched() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (episodeId: number) => markWatched(episodeId),
    onMutate: async (episodeId) => {
      await queryClient.cancelQueries({ queryKey: UP_NEXT_KEY })
      const snapshot = queryClient.getQueriesData<UpNextCard[]>({ queryKey: UP_NEXT_KEY })
      queryClient.setQueriesData<UpNextCard[]>({ queryKey: UP_NEXT_KEY }, (old) =>
        old?.map((entry) =>
          entry.episode.id === episodeId ? { ...entry, optimisticWatched: true } : entry,
        ),
      )
      return { snapshot }
    },
    onError: (_err, _episodeId, context) => {
      context?.snapshot.forEach(([key, data]) => queryClient.setQueryData(key, data))
    },
    onSuccess: (nextUp, episodeId) => {
      // Swap the card's episode to the returned next-up (or drop the card when
      // the show has no aired unwatched episodes left), across every bucket.
      queryClient.setQueriesData<UpNextCard[]>({ queryKey: UP_NEXT_KEY }, (old) =>
        old?.flatMap((entry) => {
          if (entry.episode.id !== episodeId) return [entry]
          return nextUp === null ? [] : [{ show: entry.show, episode: nextUp }]
        }),
      )
    },
  })
}
