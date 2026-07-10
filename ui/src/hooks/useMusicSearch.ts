import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addAlbum, searchMusic } from '../api/music'
import type { MusicStatus } from '../api/music'
import { LIBRARY_KEY } from './useLibrary'

/** MusicBrainz album search; disabled until the user submits a non-empty query. */
export function useMusicSearch(query: string) {
  return useQuery({
    queryKey: ['music', 'search', query] as const,
    queryFn: () => searchMusic(query),
    enabled: query !== '',
  })
}

/**
 * Adds an album by MusicBrainz id. A fresh add creates a LISTENING shelf item,
 * so the library is refreshed on success.
 */
export function useAddAlbum() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ mbid, status }: { mbid: string; status?: MusicStatus }) => addAlbum(mbid, status),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}
