import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchUpNext, markWatched } from '../api/tv'
import type { UpNextEntry } from '../api/tv'

export const UP_NEXT_KEY = ['tv', 'up-next'] as const

/**
 * Cache entry for the Up Next dashboard. `optimisticWatched` is a client-only
 * flag set while a mark-watched request is in flight so the checkmark fills in
 * immediately; it is rolled back on error and replaced by the swapped-in next
 * episode on success.
 */
export interface UpNextCard extends UpNextEntry {
  optimisticWatched?: boolean
}

export function useUpNext() {
  return useQuery<UpNextCard[]>({
    queryKey: UP_NEXT_KEY,
    queryFn: fetchUpNext,
  })
}

/**
 * One-tap watch checkmark. Optimistically marks the card
 * watched, rolls back on error, and on success swaps the card's episode to the
 * next-up episode returned by the API — or removes the card when none remains.
 */
export function useMarkWatched() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (episodeId: number) => markWatched(episodeId),
    onMutate: async (episodeId) => {
      await queryClient.cancelQueries({ queryKey: UP_NEXT_KEY })
      const previous = queryClient.getQueryData<UpNextCard[]>(UP_NEXT_KEY)
      queryClient.setQueryData<UpNextCard[]>(UP_NEXT_KEY, (old) =>
        old?.map((entry) =>
          entry.episode.id === episodeId ? { ...entry, optimisticWatched: true } : entry,
        ),
      )
      return { previous }
    },
    onError: (_err, _episodeId, context) => {
      if (context?.previous !== undefined) {
        queryClient.setQueryData(UP_NEXT_KEY, context.previous)
      }
    },
    onSuccess: (nextUp, episodeId) => {
      queryClient.setQueryData<UpNextCard[]>(UP_NEXT_KEY, (old) =>
        old?.flatMap((entry) => {
          if (entry.episode.id !== episodeId) return [entry]
          return nextUp === null ? [] : [{ show: entry.show, episode: nextUp }]
        }),
      )
    },
  })
}
